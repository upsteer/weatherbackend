package handlers

type weatherResponse struct {
	Location      string  `json:"location" bson:"location"`
	Date          string  `json:"date" bson:"date"`
	TemperatureC  float64 `json:"temperature_c" bson:"temperature_c"`
	Condition     string  `json:"condition" bson:"condition"`
	High          int     `json:"high" bson:"high"`
	Low           int     `json:"low" bson:"low"`
	WindSpeed     int     `json:"wind_speed" bson:"wind_speed"`
	Precipitation float64 `json:"precipitation" bson:"precipitation"`
	LastUpdated   string  `json:"last_updated" bson:"last_updated"`
}

type weeklyForecastDay struct {
	Date           string `json:"date"`
	MaxTemperature int    `json:"max_temperature"`
	MinTemperature int    `json:"min_temperature"`
}

type weeklyForecastResponse struct {
	Location    string              `json:"location"`
	LastUpdated string              `json:"last_updated"`
	Forecasts   []weeklyForecastDay `json:"forecasts"`
}

type dailyForecast struct {
	AccumulatedPrecipitation float64 `json:"accumulated_precipitation" bson:"accumulated_precipitation"`
	PrecipitationProbability float64 `json:"precipitation_probability" bson:"precipitation_probability"`
	AirTemperature           float64 `json:"air_temperature" bson:"air_temperature"`
	MaxTemperature           float64 `json:"max_temperature" bson:"max_temperature"`
	MinTemperature           float64 `json:"min_temperature" bson:"min_temperature"`
	RelativeHumidity         float64 `json:"relative_humidity" bson:"relative_humidity"`
	Cloud                    float64 `json:"cloud" bson:"cloud"`
	WindSpeed                float64 `json:"wind_speed" bson:"wind_speed"`
	WindDirection            float64 `json:"wind_direction" bson:"wind_direction"`
	HeatIndex                float64 `json:"heat_index" bson:"heat_index"`
	Sunrise                  string  `json:"sunrise" bson:"sunrise"`
	Sunset                   string  `json:"sunset" bson:"sunset"`
	Datetime                 string  `json:"datetime" bson:"datetime"`
	WeatherIcon              string  `json:"weather_icon" bson:"weather_icon"`
	WeatherName              string  `json:"weather_name" bson:"weather_name"`
}

type hourlyForecast struct {
	AirTemperature      float64 `json:"air_temperature" bson:"air_temperature"`
	CloudFraction       float64 `json:"cloudfraction" bson:"cloudfraction"`
	Datetime            string  `json:"datetime" bson:"datetime"`
	HourlyPrecipitation float64 `json:"hourly_precipitation" bson:"hourly_precipitation"`
	PrecipitationAmount float64 `json:"precipitation_amount" bson:"precipitation_amount"`
	RelativeHumidity    float64 `json:"relative_humidity" bson:"relative_humidity"`
	WindDirection       float64 `json:"wind_direction" bson:"wind_direction"`
	WindSpeed           float64 `json:"wind_speed" bson:"wind_speed"`
	Cloud               float64 `json:"cloud" bson:"cloud"`
	HeatIndex           float64 `json:"heat_index" bson:"heat_index"`
	WeatherIcon         string  `json:"weather_icon" bson:"weather_icon"`
	WeatherName         string  `json:"weather_name" bson:"weather_name"`
}

type weatherCreateRequest struct {
	PlaceID          string           `json:"place_id" bson:"place_id"`
	DisplayName      string           `json:"display_name" bson:"display_name"`
	CityDistrict     string           `json:"city_district" bson:"city_district"`
	Municipality     string           `json:"municipality" bson:"municipality"`
	MunicipalityNorm string           `json:"-" bson:"municipality_norm"`
	County           string           `json:"county" bson:"county"`
	Province         string           `json:"province" bson:"province"`
	Country          string           `json:"country" bson:"country"`
	Distance         float64          `json:"distance" bson:"distance"`
	ForecastDate     string           `json:"forecast_date" bson:"forecast_date"`
	ForecastDay      string           `json:"-" bson:"forecast_day"`
	Stations         []any            `json:"stations" bson:"stations"`
	DailyForecast    []dailyForecast  `json:"daily_forecast" bson:"daily_forecast"`
	HourlyForecast   []hourlyForecast `json:"hourly_forecast" bson:"hourly_forecast"`
}
