package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	_ = godotenv.Load()

	mongoURI := getEnv("MONGO_URI", "mongodb://localhost:27017/")
	mongoDB := getEnv("MONGO_DB", "weather_db")
	dbCollection := getEnv("MONGO_COLLECTION", "weather")
	port := getEnv("PORT", "5000")
	apiSecret := getEnv("API_SECRET", "dev-secret-change-me")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("failed to connect to mongo: %v", err)
	}
	defer func() {
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		if err := client.Disconnect(disconnectCtx); err != nil {
			log.Printf("failed to disconnect mongo client: %v", err)
		}
	}()

	weatherCollection := client.Database(mongoDB).Collection(dbCollection)
	mux := http.NewServeMux()

	mux.HandleFunc("/health", requireAPISecret(handleHealth, apiSecret))
	mux.HandleFunc("/weather/place", requireAPISecret(makeWeatherHandler(weatherCollection), apiSecret))
	mux.HandleFunc("/weather/create", requireAPISecret(makeCreateWeatherHandler(weatherCollection), apiSecret))

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("server listening on http://localhost:%s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to encode json response: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
