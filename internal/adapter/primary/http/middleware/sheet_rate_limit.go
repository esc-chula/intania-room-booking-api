package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"
)

const sheetRate = 10 // 600 requests per minute per IP, across all three routes.
const sheetBurst = 600

type sheetBucket struct {
	tokens float64
	last   time.Time
}

// SheetRateLimit applies a per-IP token bucket to the Sheet endpoints.
// The buckets are local to an instance; use an edge limiter if instances scale out.
func SheetRateLimit() func(http.Handler) http.Handler {
	var mu sync.Mutex
	buckets := make(map[string]sheetBucket)
	lastCleanup := time.Now()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := time.Now()
			ip := sheetClientIP(r)
			mu.Lock()
			if now.Sub(lastCleanup) >= time.Minute {
				for key, bucket := range buckets {
					if now.Sub(bucket.last) > 2*time.Minute {
						delete(buckets, key)
					}
				}
				lastCleanup = now
			}
			bucket, ok := buckets[ip]
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
			buckets[ip] = bucket
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

func sheetClientIP(r *http.Request) string {
	// Cloud Run appends the connecting client after any caller-supplied XFF entries.
	if os.Getenv("K_SERVICE") != "" {
		values := r.Header.Values("X-Forwarded-For")
		if len(values) == 1 {
			parts := strings.Split(values[0], ",")
			if ip, err := netip.ParseAddr(strings.TrimSpace(parts[len(parts)-1])); err == nil {
				return ip.Unmap().String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "unknown"
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Unmap().String()
	}
	return "unknown"
}
