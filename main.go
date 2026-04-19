package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"weatherbackend/handlers"
	"weatherbackend/middleware"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func main() {
	_ = godotenv.Load()
	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mongoURI := getEnv("MONGO_URI", "")
	mongoDB := getEnv("MONGO_DB", "weather_db")
	dbCollection := getEnv("MONGO_COLLECTION", "weather")
	dbMunicipalityCollection := getEnv("MUNICIPALITY_COLLECTION", "municipality")
	port := getEnv("PORT", "5000")
	apiSecret := getEnv("API_SECRET", "dev-secret-change-me")
	weatherFetchURL := getEnv("WEATHER_FETCH_URL", "")
	weatherCreateURL := getEnv("WEATHER_CREATE_URL", "http://localhost:"+port+"/weather/create")

	if mongoURI == "" {
		log.Fatal("MONGO_URI environment variable is required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("failed to connect to mongo: %v", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("failed to ping mongo: %v", err)
	}
	defer func() {
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		if err := client.Disconnect(disconnectCtx); err != nil {
			log.Printf("failed to disconnect mongo client: %v", err)
		}
	}()

	weatherCollection := client.Database(mongoDB).Collection(dbCollection)
	municipalityCollection := client.Database(mongoDB).Collection(dbMunicipalityCollection)
	indexCtx, indexCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer indexCancel()
	if err := handlers.EnsureWeatherIndexes(indexCtx, weatherCollection); err != nil {
		log.Fatalf("failed to ensure weather indexes: %v", err)
	}

	handlers.StartDailyWeatherFetchJob(appCtx, municipalityCollection, weatherFetchURL, weatherCreateURL, apiSecret)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", middleware.RequireAPISecret(handlers.HandleHealth, apiSecret))
	mux.HandleFunc("/weather/place", middleware.RequireAPISecret(handlers.MakeWeatherHandler(weatherCollection), apiSecret))
	mux.HandleFunc("/weather/weeklyForecast", middleware.RequireAPISecret(handlers.MakeWeeklyForecastHandler(weatherCollection), apiSecret))
	mux.HandleFunc("/weather/create", middleware.RequireAPISecret(handlers.MakeCreateWeatherHandler(weatherCollection), apiSecret))
	mux.HandleFunc("/weather/fetch-now", middleware.RequireAPISecret(handlers.MakeManualDailyWeatherFetchHandler(municipalityCollection, weatherFetchURL, weatherCreateURL, apiSecret), apiSecret))

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

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
