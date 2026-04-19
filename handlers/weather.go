package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const maxWeeklyForecastDays = 7

var nepalLocation = loadNepalLocation()

func loadNepalLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err == nil {
		return loc
	}

	return time.FixedZone("NPT", 5*60*60+45*60)
}

func MakeWeatherHandler(weatherCollection *mongo.Collection) http.HandlerFunc {
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

		targetTime := time.Now().UTC()
		date := targetTime.Format(time.DateOnly)

		queryCtx, queryCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer queryCancel()

		weather, err := fetchWeatherWithFallback(queryCtx, weatherCollection, location, date)
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

		currentDayWeather := getTodaysWeather(weather.DailyForecast, date)

		if currentDayWeather == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "weather data for the requested date is not available yet"})
			return
		}

		wR := weatherResponse{
			Location:      weather.DisplayName,
			Date:          formatDateTimeForNepalResponse(nearestHourly.Datetime),
			TemperatureC:  nearestHourly.AirTemperature,
			Condition:     nearestHourly.WeatherName,
			High:          int(currentDayWeather.MaxTemperature),
			Low:           int(currentDayWeather.MinTemperature),
			WindSpeed:     int(nearestHourly.WindSpeed),
			Precipitation: nearestHourly.PrecipitationAmount,
			LastUpdated:   formatDateTimeForNepalResponse(weather.ForecastDate),
		}

		writeJSON(w, http.StatusOK, wR)
	}
}

func MakeWeeklyForecastHandler(weatherCollection *mongo.Collection) http.HandlerFunc {
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

		date := time.Now().UTC().Format(time.DateOnly)

		queryCtx, queryCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer queryCancel()

		weather, err := fetchWeatherWithFallback(queryCtx, weatherCollection, location, date)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "no weather found for '" + location + "' on '" + date + "'"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}

		if len(weather.DailyForecast) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no daily forecast available for the requested location"})
			return
		}

		sortedForecasts := sortedDailyForecast(weather.DailyForecast)
		limit := min(len(sortedForecasts), maxWeeklyForecastDays)

		forecasts := make([]weeklyForecastDay, 0, limit)
		for _, forecast := range sortedForecasts[:limit] {
			forecasts = append(forecasts, weeklyForecastDay{
				Date:           strings.TrimSpace(forecast.Datetime),
				MaxTemperature: int(forecast.MaxTemperature),
				MinTemperature: int(forecast.MinTemperature),
			})
		}

		writeJSON(w, http.StatusOK, weeklyForecastResponse{
			Location:    weather.DisplayName,
			LastUpdated: formatDateTimeForNepalResponse(weather.ForecastDate),
			Forecasts:   forecasts,
		})
	}
}

func formatDateTimeForNepalResponse(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return trimmed
	}

	parsed, err := parseTimeValue(trimmed)
	if err != nil {
		return trimmed
	}

	return parsed.In(nepalLocation).Format(time.RFC3339)
}

func buildWeatherLookupFilter(location, date string) bson.M {
	trimmedLocation := strings.TrimSpace(location)
	normalizedLocation := normalizeMunicipality(trimmedLocation)
	legacyDateFilter := bson.A{
		bson.M{"forecast_date": date},
		bson.M{"forecast_date": bson.M{"$regex": "^" + date + "T"}},
	}

	return bson.M{
		"$or": bson.A{
			bson.M{
				"municipality_norm": normalizedLocation,
				"forecast_day":      date,
			},
			bson.M{
				"municipality": bson.M{
					"$regex":   "^" + regexp.QuoteMeta(trimmedLocation) + "$",
					"$options": "i",
				},
				"$or": legacyDateFilter,
			},
		},
	}
}

func fetchWeatherByLocationAndDate(ctx context.Context, weatherCollection *mongo.Collection, location, date string) (weatherCreateRequest, error) {
	var weather weatherCreateRequest
	err := weatherCollection.FindOne(ctx, buildWeatherLookupFilter(location, date)).Decode(&weather)
	if err != nil {
		return weatherCreateRequest{}, err
	}

	return weather, nil
}

func fetchWeatherWithFallback(ctx context.Context, weatherCollection *mongo.Collection, location, date string) (weatherCreateRequest, error) {
	weather, err := fetchWeatherByLocationAndDate(ctx, weatherCollection, location, date)
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return weather, err
	}

	parsed, parseErr := time.Parse(time.DateOnly, date)
	if parseErr != nil {
		return weatherCreateRequest{}, err
	}

	previousDate := parsed.AddDate(0, 0, -1).Format(time.DateOnly)
	return fetchWeatherByLocationAndDate(ctx, weatherCollection, location, previousDate)
}

func sortedDailyForecast(dailyForecasts []dailyForecast) []dailyForecast {
	forecasts := make([]dailyForecast, 0, len(dailyForecasts))
	for _, forecast := range dailyForecasts {
		date := strings.TrimSpace(forecast.Datetime)
		if date == "" {
			continue
		}
		forecasts = append(forecasts, forecast)
	}

	sort.Slice(forecasts, func(i, j int) bool {
		iDate := strings.TrimSpace(forecasts[i].Datetime)
		jDate := strings.TrimSpace(forecasts[j].Datetime)

		iParsed, iErr := time.Parse(time.DateOnly, iDate)
		jParsed, jErr := time.Parse(time.DateOnly, jDate)

		if iErr == nil && jErr == nil {
			return iParsed.Before(jParsed)
		}

		return iDate < jDate
	})

	return forecasts
}

func getTodaysWeather(dailyForecasts []dailyForecast, targetDate string) *dailyForecast {
	for _, forecast := range dailyForecasts {
		if strings.TrimSpace(forecast.Datetime) == targetDate {
			return &forecast
		}
	}
	return nil
}

func findNearestHourlyForecast(entries []hourlyForecast, targetTime time.Time) (hourlyForecast, bool) {
	utcTarget := targetTime.UTC()
	targetDay := time.Date(utcTarget.Year(), utcTarget.Month(), utcTarget.Day(), 0, 0, 0, 0, time.UTC)

	var best hourlyForecast
	hasBest := false
	bestPriority := 0
	var bestDiff time.Duration

	for _, entry := range entries {
		entryTime, err := parseTimeValue(strings.TrimSpace(entry.Datetime))
		if err != nil {
			continue
		}
		utcEntry := entryTime.UTC()
		entryDay := time.Date(utcEntry.Year(), utcEntry.Month(), utcEntry.Day(), 0, 0, 0, 0, time.UTC)

		delta := utcEntry.Sub(utcTarget)
		priority := 0
		diff := delta

		switch {
		case entryDay.Equal(targetDay) && delta >= 0:
			// Best case: upcoming forecast for the current Nepal date.
			priority = 0
		case entryDay.Equal(targetDay):
			// Same day but in the past.
			priority = 1
		case entryDay.After(targetDay):
			// Future date (typically t+1).
			priority = 2
		default:
			// Previous date(s), keep as last-resort fallback.
			priority = 3
		}

		if diff < 0 {
			diff = -diff
		}

		if !hasBest || priority < bestPriority || (priority == bestPriority && diff < bestDiff) {
			best = entry
			bestPriority = priority
			bestDiff = diff
			hasBest = true
		}
	}

	return best, hasBest
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

func normalizeMunicipality(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeForecastDay(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("forecast date is required")
	}

	if parsed, err := time.Parse(time.DateOnly, trimmed); err == nil {
		return parsed.Format(time.DateOnly), nil
	}

	parsed, err := parseTimeValue(trimmed)
	if err != nil {
		return "", errors.New("forecast_date must be YYYY-MM-DD or RFC3339")
	}

	return parsed.UTC().Format(time.DateOnly), nil
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), "E11000")
}

func EnsureWeatherIndexes(ctx context.Context, weatherCollection *mongo.Collection) error {
	_, err := weatherCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "municipality_norm", Value: 1},
			{Key: "forecast_day", Value: 1},
		},
		Options: options.Index().SetName("uniq_municipality_forecast_day").SetUnique(true),
	})

	return err
}

func MakeCreateWeatherHandler(weatherCollection *mongo.Collection) http.HandlerFunc {
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
		if strings.TrimSpace(payload.Municipality) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "municipality is required"})
			return
		}

		normalizedDay, err := normalizeForecastDay(payload.ForecastDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		payload.Municipality = strings.TrimSpace(payload.Municipality)
		payload.ForecastDate = strings.TrimSpace(payload.ForecastDate)
		payload.MunicipalityNorm = normalizeMunicipality(payload.Municipality)
		payload.ForecastDay = normalizedDay

		insertCtx, insertCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer insertCancel()

		filter := bson.M{
			"municipality_norm": payload.MunicipalityNorm,
			"forecast_day":      payload.ForecastDay,
		}

		result, err := weatherCollection.UpdateOne(insertCtx, filter, bson.M{"$set": payload}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			if isDuplicateKeyError(err) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "weather entry already exists for this municipality and date"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}

		status := http.StatusOK
		if result.UpsertedCount > 0 {
			status = http.StatusCreated
		}

		writeJSON(w, status, payload)
	}
}
