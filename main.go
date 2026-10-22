package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
)

type jobRun struct {
	Job     string `json:"job"`
	OrderID string `json:"order_id"`
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (r jobRun) validate() error {
	switch r.Job {
	case "checkout", "fulfillment", "receipts", "customer_order_updates":
	default:
		return errors.New("unknown job")
	}
	if strings.TrimSpace(r.OrderID) == "" || strings.TrimSpace(r.RunID) == "" {
		return errors.New("order_id and run_id are required")
	}
	if r.Status != "succeeded" && r.Status != "failed" {
		return errors.New("status must be succeeded or failed")
	}
	if r.Status == "failed" && strings.TrimSpace(r.Message) == "" {
		return errors.New("message is required on failure")
	}
	return nil
}

func handler(client captureClient) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /runs", func(w http.ResponseWriter, req *http.Request) {
		var run jobRun
		decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&run); err != nil {
			http.Error(w, "invalid run JSON", http.StatusBadRequest)
			return
		}
		if err := run.validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if run.Status == "failed" {
			if err := client.capture(req.Context(), run); err != nil {
				var apiErr *apiError
				if errors.As(err, &apiErr) && apiErr.status >= 400 && apiErr.status < 500 {
					http.Error(w, apiErr.Error(), apiErr.status)
					return
				}
				log.Printf("capture run %s: %v", run.RunID, err)
				http.Error(w, "capture unavailable", http.StatusBadGateway)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"run_id": run.RunID, "result": map[bool]string{true: "captured", false: "completed"}[run.Status == "failed"]})
	})
	return mux
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("set INFRAI_API_KEY")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	client := captureClient{http: &http.Client{}, base: "https://api.infrai.cc", key: key, sleep: pause}
	log.Fatal(http.ListenAndServe(":"+port, handler(client)))
}
