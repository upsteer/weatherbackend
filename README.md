# Weather Backend (Go + MongoDB)

This project shows a basic Go HTTP service that fetches weather data from a MongoDB database.

## What You Will Build

- A Go server with a single endpoint: `GET /weather/{city}`
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
PORT=5000
API_SECRET=dev-secret-change-me
```

Use your MongoDB Atlas SRV connection string for `MONGO_URI`.
If your password contains special characters, URL-encode it before placing it in the URI.

## 3. Run the Service

```bash
go run .
```

The service will start on:

```text
http://localhost:5000
```

## 4. Test the Endpoints

Health check:

```bash
curl -H "X-API-Secret: dev-secret-change-me" http://localhost:5000/health
```

Get weather by city:

```bash
curl -H "X-API-Secret: dev-secret-change-me" http://localhost:5000/weather/london
```

Example response:

```json
{
	"city": "london",
	"temperature_c": 14,
	"condition": "Cloudy"
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
