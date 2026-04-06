package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func makeWeatherHandler(weatherCollection *mongo.Collection) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		location := strings.TrimSpace(r.URL.Query().Get("location"))
		if location == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "location is required"})
			return
		}

		targetTimeRaw := strings.TrimSpace(r.URL.Query().Get("target_time"))
		if targetTimeRaw == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target_time is required"})
			return
		}

		targetTime, err := parseTimeValue(targetTimeRaw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target_time must be a valid ISO-8601 datetime"})
			return
		}

		date := strings.TrimSpace(r.URL.Query().Get("date"))
		if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("current")), "true") {
			date = time.Now().Format(time.DateOnly)
		} else if date == "" {
			date = targetTime.Format(time.DateOnly)
		}

		var weather weatherCreateRequest
		queryCtx, queryCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer queryCancel()

		err = weatherCollection.FindOne(queryCtx, bson.M{"municipality": location, "daily_forecast.datetime": date}).Decode(&weather)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "no weather found for '" + location + "' on '" + date + "'"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}

		nearestHourly, ok := findNearestHourlyForecast(weather.HourlyForecast, targetTime)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no hourly forecast found for the requested time"})
			return
		}

		wR := weatherResponse{
			Location:     weather.DisplayName,
			Date:         nearestHourly.Datetime,
			TemperatureC: int(nearestHourly.AirTemperature),
			Condition:    nearestHourly.WeatherName,
		}

		writeJSON(w, http.StatusOK, wR)
	}
}

func findNearestHourlyForecast(entries []hourlyForecast, targetTime time.Time) (hourlyForecast, bool) {
	var nearest hourlyForecast
	hasNearest := false
	var smallestDiff time.Duration

	for _, entry := range entries {
		entryTime, err := parseTimeValue(strings.TrimSpace(entry.Datetime))
		if err != nil {
			continue
		}

		diff := entryTime.Sub(targetTime)
		if diff < 0 {
			diff = -diff
		}

		if !hasNearest || diff < smallestDiff {
			nearest = entry
			smallestDiff = diff
			hasNearest = true
		}
	}

	return nearest, hasNearest
}

func parseTimeValue(raw string) (time.Time, error) {
	layouts := []string{time.RFC3339Nano, time.RFC3339}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, errors.New("invalid datetime")
}

func makeCreateWeatherHandler(weatherCollection *mongo.Collection) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload weatherCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if strings.TrimSpace(payload.PlaceID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "place_id is required"})
			return
		}
		if strings.TrimSpace(payload.ForecastDate) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "forecast_date is required"})
			return
		}

		insertCtx, insertCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer insertCancel()

		_, err := weatherCollection.InsertOne(insertCtx, payload)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error, err: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, payload)
	}
}
