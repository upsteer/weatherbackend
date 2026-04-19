# Weather Backend (Go + MongoDB)

This project shows a basic Go HTTP service that fetches weather data from a MongoDB database.

## What You Will Build

- A Go server with weather endpoints for point-in-time and weekly forecasts
- A MongoDB database with a `weather` collection
- API responses returned as JSON

## Tech Stack

- Go 1.22+
- Standard library (`net/http`)
- MongoDB Go Driver
- MongoDB (local or MongoDB Atlas)

## Project Structure

```text
weatherbackend/
├── main.go
├── health_handler.go
├── weather_handler.go
├── go.mod
├── go.sum
├── .env.example
├── .env
└── README.md
```

## 1. Install Dependencies

```bash
go mod tidy
```

## 2. Add Environment Variables

Copy the template and adjust values as needed:

```bash
cp .env.example .env
```

Default `.env.example`:

```env
MONGO_URI=mongodb+srv://<username>:<password>@<cluster-host>/<db-name>?retryWrites=true&w=majority
MONGO_DB=weather_db
MONGO_COLLECTION=weather
MUNICIPALITY_COLLECTION=municipality
PORT=5000
API_SECRET=dev-secret-change-me
WEATHER_FETCH_URL=
WEATHER_CREATE_URL=http://localhost:5000/weather/create
```

Use your MongoDB Atlas SRV connection string for `MONGO_URI`.
If your password contains special characters, URL-encode it before placing it in the URI.
Set `WEATHER_FETCH_URL` to enable a daily background weather fetch at around `03:00 UTC`.
Set `WEATHER_CREATE_URL` to control where fetched payloads are posted for DB upsert (default: local `/weather/create`).
If left empty, the background job stays disabled.

## 3. Run the Service

```bash
go run .
```

The service will start on:

```text
http://localhost:5000
```

When `WEATHER_FETCH_URL` is set, the server also starts a background job that sends GET requests once per day near `03:00 UTC`.
On each run, it loads up to 3 records from the `municipality` collection in `weather_db` and calls the URL with query params `lat` and `lng` for each place.
Each successful upstream payload is then posted to `/weather/create` (or `WEATHER_CREATE_URL`) so entries are inserted/upserted using the same logic as the create endpoint.
If `API_SECRET` is configured, it is forwarded as the `X-API-Secret` header on both upstream fetch and create calls.

## 4. Test the Endpoints

Health check:

```bash
curl -H "X-API-Secret: dev-secret-change-me" http://localhost:5000/health
```

Get weather by location:

```bash
curl -H "X-API-Secret: dev-secret-change-me" "http://localhost:5000/weather/place?location=Kathmandu"
```

Get 7-day daily max/min forecast:

```bash
curl -H "X-API-Secret: dev-secret-change-me" "http://localhost:5000/weather/weeklyForecast?location=Kathmandu"
```

Trigger the municipality background fetch manually (runs immediately once for up to 3 municipalities):

```bash
curl -X POST -H "X-API-Secret: dev-secret-change-me" "http://localhost:5000/weather/fetch-now"
```

Example response:

```json
{
	"location": "Kathmandu",
	"last_updated": "2026-04-12T00:00:00Z",
	"forecasts": [
		{
			"date": "2026-04-12",
			"max_temperature": 25,
			"min_temperature": 12
		}
	]
}
```

## Common Issues

- Connection error to MongoDB:
	- Verify your Atlas SRV URI, database user/password, and Atlas network access list.
- Missing Go modules:
	- Run `go mod tidy` and try `go run .` again.
- Unauthorized responses:
	- Ensure the request includes the `X-API-Secret` header matching `API_SECRET`.
- Empty API response:
	- Confirm you inserted sample documents into the `weather_db.weather` collection.

## Next Improvements

- Add POST/PUT/DELETE endpoints
- Add request validation for request payloads
- Add Docker support for Go + MongoDB
- Add unit tests with Go's `testing` package

## Build
Login to Docker, then run
Run
`docker build --platform linux/amd64 -t upsteer/weatherbackend:<version_number> .`

Then push to docker hub:
docker push upsteer/weatherbackend:<version_number>
