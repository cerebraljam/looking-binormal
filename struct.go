package main

type EventSpec struct {
	Organization string `json:"org"`
	Source       string `json:"source"`
	Timestamp    string `json:"timestamp"`
	Population   string `json:"pop"`
	Identifier   string `json:"id"`
	Action       string `json:"action"`
}

type AliveResponseSpec struct {
	Runtime int64  `json:"runtime"`
	Status  string `json:"status"`
}

type DiscreteResponseSpec struct {
	Runtime         int64   `json:"runtime"`
	Score           float64 `json:"score"`
	Count           int64   `json:"count"`
	Zscore          float64 `json:"zscore"`
	Source          string  `json:"source"`
	Population      string  `json:"pop"`
	Timestamp       string  `json:"timestamp"`
	Identifier      string  `json:"id"`
	Action          string  `json:"action"`
	ActionDeviation float64 `json:"deviation"`
}
