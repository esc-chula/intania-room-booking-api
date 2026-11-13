package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"
)

const sheetRate = 10 // 600 requests per minute per user, across all three routes.
const sheetBurst = 600

type sheetBucket struct {
	tokens float64
	last   time.Time
}

// SheetRateLimit applies a per-user token bucket to the Sheet endpoints.
// The buckets are local to an instance.
func SheetRateLimit(userID func(context.Context) (string, bool)) func(http.Handler) http.Handler {
	var mu sync.Mutex
	buckets := make(map[string]sheetBucket)
	lastCleanup := time.Now()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := time.Now()
			id, ok := userID(r.Context())
			if !ok || id == "" {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid authentication cookie"}`))
				return
			}
			mu.Lock()
			if now.Sub(lastCleanup) >= time.Minute {
				for key, bucket := range buckets {
					if now.Sub(bucket.last) > 2*time.Minute {
						delete(buckets, key)
					}
				}
				lastCleanup = now
			}
			bucket, ok := buckets[id]
			if !ok {
				bucket.tokens = sheetBurst
			} else {
				bucket.tokens = min(sheetBurst, bucket.tokens+now.Sub(bucket.last).Seconds()*sheetRate)
			}
			bucket.last = now
			allowed := bucket.tokens >= 1
			if allowed {
				bucket.tokens--
			}
			buckets[id] = bucket
			mu.Unlock()
			if !allowed {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
