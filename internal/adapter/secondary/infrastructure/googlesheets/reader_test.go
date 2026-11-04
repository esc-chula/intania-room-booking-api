package googlesheets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func TestList(t *testing.T) {
	row := make([]any, 19)
	for i := range row {
		row[i] = ""
	}
	row[0], row[5], row[13] = "123", "Requester", "Room A"
	row[15], row[16], row[17], row[18] = "2026-11-04", "2026-11-04", "09:00", "10:00"
	for _, tc := range []struct {
		name       string
		values     [][]any
		status     int
		wantLength int
		wantError  bool
	}{
		{name: "full and short rows", values: [][]any{row, {"456"}}, wantLength: 2},
		{name: "empty sheet", wantLength: 0},
		{name: "upstream error", status: http.StatusServiceUnavailable, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/v4/spreadsheets/test-id/values/Bookings!A9:AM") || r.URL.Query().Get("alt") != "json" || r.URL.Query().Get("valueRenderOption") != "" {
					t.Errorf("unexpected Sheets request: %s %s", r.Method, r.URL)
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"values": tc.values})
			}))
			defer server.Close()
			service, err := sheets.NewService(context.Background(), option.WithEndpoint(server.URL+"/"), option.WithoutAuthentication())
			if err != nil {
				t.Fatal(err)
			}
			reader := &Reader{service: service, spreadsheetID: "test-id", readRange: "Bookings!A9:AM"}
			bookings, err := reader.List(context.Background())
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantError {
				return
			}
			if bookings == nil || len(bookings) != tc.wantLength {
				t.Fatalf("unexpected bookings: %+v", bookings)
			}
			if tc.wantLength == 2 {
				if b := bookings[0]; b.ID != "123" || b.Name != "Requester" || b.Room != "Room A" || b.StartDate != "2026-11-04" || b.EndDate != "2026-11-04" || b.StartTime != "09:00" || b.EndTime != "10:00" {
					t.Fatalf("wrong column mapping: %+v", b)
				}
				if b := bookings[1]; b.ID != "456" || b.Name != "" || b.Room != "" || b.StartDate != "" || b.EndDate != "" || b.StartTime != "" || b.EndTime != "" {
					t.Fatalf("short row not preserved: %+v", b)
				}
			}
		})
	}
}
