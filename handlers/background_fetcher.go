package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	defaultFetchHourUTC      = 3
	defaultFetchTimeout      = 30 * time.Second
	maxErrorResponseBodySize = 4096
	maxSuccessResponseBody   = 2 * 1024 * 1024
	dailyFetchLocationLimit  = 3
	perLocationDelay         = 1 * time.Second
)

type municipalityCoordinates struct {
	Municipality string
	Lat          float64
	Lng          float64
}

type dailyFetchResult struct {
	Requested    int `json:"requested"`
	Succeeded    int `json:"succeeded"`
	Failed       int `json:"failed"`
	FetchFailed  int `json:"fetch_failed"`
	CreateFailed int `json:"create_failed"`
}

type upstreamWeatherResponse struct {
	PlaceID        string           `json:"place_id"`
	DisplayName    string           `json:"display_name"`
	CityDistrict   string           `json:"city_district"`
	Municipality   *string          `json:"municipality"`
	County         string           `json:"county"`
	Province       string           `json:"province"`
	Country        string           `json:"country"`
	Distance       float64          `json:"distance"`
	ForecastDate   string           `json:"forecast_date"`
	Stations       []any            `json:"stations"`
	DailyForecast  []dailyForecast  `json:"daily_forecast"`
	HourlyForecast []hourlyForecast `json:"hourly_forecast"`
}

// StartDailyWeatherFetchJob starts a non-blocking background job that fetches weather for 3 municipalities once per day at 03:00 UTC.
func StartDailyWeatherFetchJob(ctx context.Context, municipalityCollection *mongo.Collection, fetchURL, createURL, apiSecret string) {
	trimmedURL := strings.TrimSpace(fetchURL)
	trimmedCreateURL := strings.TrimSpace(createURL)
	if trimmedURL == "" {
		log.Printf("daily weather fetch job is disabled: WEATHER_FETCH_URL is not set")
		return
	}
	if trimmedCreateURL == "" {
		log.Printf("daily weather fetch job is disabled: WEATHER_CREATE_URL is not set")
		return
	}
	if municipalityCollection == nil {
		log.Printf("daily weather fetch job is disabled: municipality collection is not configured")
		return
	}

	client := &http.Client{Timeout: defaultFetchTimeout}

	go func() {
		for {
			waitFor := durationUntilNextUTCHour(defaultFetchHourUTC)
			log.Printf("next weather fetch scheduled in %s at %s", waitFor.Round(time.Second), time.Now().UTC().Add(waitFor).Format(time.RFC3339))

			timer := time.NewTimer(waitFor)
			select {
			case <-ctx.Done():
				timer.Stop()
				log.Printf("daily weather fetch job stopped")
				return
			case <-timer.C:
			}

			result, err := runDailyWeatherFetchOnce(ctx, client, municipalityCollection, trimmedURL, trimmedCreateURL, apiSecret)
			if err != nil {
				log.Printf("daily weather fetch failed: %v", err)
				continue
			}

			if result.Succeeded == 0 {
				log.Printf("daily weather fetch finished with no successful requests")
				continue
			}

			log.Printf("daily weather fetch completed successfully for %d/%d municipalities", result.Succeeded, result.Requested)
		}
	}()
}

// MakeManualDailyWeatherFetchHandler returns an HTTP handler that triggers a one-time weather fetch for 3 municipalities.
func MakeManualDailyWeatherFetchHandler(municipalityCollection *mongo.Collection, fetchURL, createURL, apiSecret string) http.HandlerFunc {
	trimmedURL := strings.TrimSpace(fetchURL)
	trimmedCreateURL := strings.TrimSpace(createURL)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if trimmedURL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "WEATHER_FETCH_URL is not set"})
			return
		}
		if trimmedCreateURL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "WEATHER_CREATE_URL is not set"})
			return
		}
		if municipalityCollection == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "municipality collection is not configured"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), defaultFetchTimeout*time.Duration(dailyFetchLocationLimit+1))
		defer cancel()

		client := &http.Client{Timeout: defaultFetchTimeout}
		result, err := runDailyWeatherFetchOnce(ctx, client, municipalityCollection, trimmedURL, trimmedCreateURL, apiSecret)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		status := http.StatusOK
		if result.Succeeded == 0 {
			status = http.StatusBadGateway
		}

		writeJSON(w, status, result)
	}
}

func runDailyWeatherFetchOnce(ctx context.Context, client *http.Client, municipalityCollection *mongo.Collection, fetchURL, createURL, apiSecret string) (dailyFetchResult, error) {
	locations, err := loadMunicipalityCoordinates(ctx, municipalityCollection, dailyFetchLocationLimit)
	if err != nil {
		return dailyFetchResult{}, fmt.Errorf("failed to load municipalities: %w", err)
	}
	if len(locations) == 0 {
		return dailyFetchResult{}, fmt.Errorf("no municipalities with lat/lng found")
	}

	result := dailyFetchResult{Requested: len(locations)}
	for i, location := range locations {
		if i > 0 {
			timer := time.NewTimer(perLocationDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return result, ctx.Err()
			case <-timer.C:
			}
		}

		requestURL, err := withLatLngQuery(fetchURL, location.Lat, location.Lng)
		if err != nil {
			result.Failed++
			result.FetchFailed++
			log.Printf("daily weather fetch failed to build URL for %q: %v", location.Municipality, err)
			continue
		}

		payload, err := fetchWeatherFromURL(ctx, client, requestURL, location.Municipality)
		if err != nil {
			result.Failed++
			result.FetchFailed++
			log.Printf("daily weather fetch failed for %q (lat=%f,lng=%f): %v", location.Municipality, location.Lat, location.Lng, err)
			continue
		}

		if err := createWeatherEntry(ctx, client, createURL, apiSecret, payload); err != nil {
			result.Failed++
			result.CreateFailed++
			log.Printf("daily weather create failed for %q (place_id=%s): %v", location.Municipality, payload.PlaceID, err)
			continue
		}

		result.Succeeded++
		log.Printf("daily weather fetch completed for %q (lat=%f,lng=%f)", location.Municipality, location.Lat, location.Lng)
	}

	return result, nil
}

func loadMunicipalityCoordinates(ctx context.Context, municipalityCollection *mongo.Collection, limit int) ([]municipalityCoordinates, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, defaultFetchTimeout)
	defer cancel()

	findOptions := options.Find().
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "municipality", Value: 1}})

	cursor, err := municipalityCollection.Find(fetchCtx, bson.M{}, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(fetchCtx)

	locations := make([]municipalityCoordinates, 0, limit)
	for cursor.Next(fetchCtx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}

		lat, latOK := getFloatFromDoc(doc, "lat", "latitude")
		lng, lngOK := getFloatFromDoc(doc, "lng", "longitude")
		if !latOK || !lngOK {
			continue
		}

		municipality, _ := doc["municipality"].(string)
		municipality = strings.TrimSpace(municipality)
		if municipality == "" {
			municipality = "unknown"
		}

		locations = append(locations, municipalityCoordinates{
			Municipality: municipality,
			Lat:          lat,
			Lng:          lng,
		})
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	if len(locations) > limit {
		return locations[:limit], nil
	}

	return locations, nil
}

func getFloatFromDoc(doc bson.M, keys ...string) (float64, bool) {
	for _, key := range keys {
		raw, ok := doc[key]
		if !ok {
			continue
		}

		switch value := raw.(type) {
		case float64:
			return value, true
		case float32:
			return float64(value), true
		case int:
			return float64(value), true
		case int32:
			return float64(value), true
		case int64:
			return float64(value), true
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err == nil {
				return parsed, true
			}
		}
	}

	return 0, false
}

func withLatLngQuery(baseURL string, lat, lng float64) (string, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	query := parsedURL.Query()
	query.Set("lat", strconv.FormatFloat(lat, 'f', 6, 64))
	query.Set("lng", strconv.FormatFloat(lng, 'f', 6, 64))
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String(), nil
}

func durationUntilNextUTCHour(targetHour int) time.Duration {
	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), targetHour, 0, 0, 0, time.UTC)

	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}

	return next.Sub(now)
}

func fetchWeatherFromURL(parent context.Context, client *http.Client, fetchURL, fallbackMunicipality string) (weatherCreateRequest, error) {
	fetchCtx, cancel := context.WithTimeout(parent, defaultFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return weatherCreateRequest{}, fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return weatherCreateRequest{}, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBodySize))
		if readErr != nil {
			return weatherCreateRequest{}, fmt.Errorf("request failed with status %d and unreadable body: %w", resp.StatusCode, readErr)
		}
		return weatherCreateRequest{}, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSuccessResponseBody))
	if err != nil {
		return weatherCreateRequest{}, fmt.Errorf("read response body: %w", err)
	}

	var upstream upstreamWeatherResponse
	if err := json.Unmarshal(body, &upstream); err != nil {
		return weatherCreateRequest{}, fmt.Errorf("decode weather payload: %w", err)
	}

	payload := mapUpstreamPayload(upstream, fallbackMunicipality)
	if strings.TrimSpace(payload.PlaceID) == "" {
		return weatherCreateRequest{}, errors.New("upstream payload missing place_id")
	}
	if strings.TrimSpace(payload.Municipality) == "" {
		return weatherCreateRequest{}, errors.New("upstream payload missing municipality")
	}
	if strings.TrimSpace(payload.ForecastDate) == "" {
		return weatherCreateRequest{}, errors.New("upstream payload missing forecast_date")
	}

	return payload, nil

}

func mapUpstreamPayload(source upstreamWeatherResponse, fallbackMunicipality string) weatherCreateRequest {
	municipality := ""
	if source.Municipality != nil {
		municipality = strings.TrimSpace(*source.Municipality)
	}
	if municipality == "" {
		municipality = strings.TrimSpace(fallbackMunicipality)
	}

	return weatherCreateRequest{
		PlaceID:        strings.TrimSpace(source.PlaceID),
		DisplayName:    strings.TrimSpace(source.DisplayName),
		CityDistrict:   strings.TrimSpace(source.CityDistrict),
		Municipality:   municipality,
		County:         strings.TrimSpace(source.County),
		Province:       strings.TrimSpace(source.Province),
		Country:        strings.TrimSpace(source.Country),
		Distance:       source.Distance,
		ForecastDate:   strings.TrimSpace(source.ForecastDate),
		Stations:       source.Stations,
		DailyForecast:  source.DailyForecast,
		HourlyForecast: source.HourlyForecast,
	}
}

func createWeatherEntry(parent context.Context, client *http.Client, createURL, apiSecret string, payload weatherCreateRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal create payload: %w", err)
	}

	createCtx, cancel := context.WithTimeout(parent, defaultFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(createCtx, http.MethodPost, createURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if secret := strings.TrimSpace(apiSecret); secret != "" {
		req.Header.Set("X-API-Secret", secret)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("perform create request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		errBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBodySize))
		if readErr != nil {
			return fmt.Errorf("create failed with status %d and unreadable body: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("create failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	return nil
}
