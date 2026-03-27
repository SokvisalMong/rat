package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
)

const tmdbBaseURL = "https://api.themoviedb.org/3"

type TMDBMultiSearchResponse struct {
	Results []TMDBResult `json:"results"`
}

type TMDBResult struct {
	ID           int     `json:"id"`
	MediaType    string  `json:"media_type"`
	Title        string  `json:"title"`        // For movies
	Name         string  `json:"name"`         // For TV shows
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	ReleaseDate  string  `json:"release_date"`   // For movies
	FirstAirDate string  `json:"first_air_date"` // For TV shows
	VoteAverage  float64 `json:"vote_average"`
	GenreIDs     []int   `json:"genre_ids"` // Added for select menu
}

type TMDBGenre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type TMDBTVDetails struct {
	ID               int         `json:"id"`
	Name             string      `json:"name"`
	Overview         string      `json:"overview"`
	PosterPath       string      `json:"poster_path"`
	FirstAirDate     string      `json:"first_air_date"`
	VoteAverage      float64     `json:"vote_average"`
	NumberOfEpisodes int         `json:"number_of_episodes"`
	NumberOfSeasons  int         `json:"number_of_seasons"`
	EpisodeRunTime   []int       `json:"episode_run_time"` // Added episode_run_time
	Genres           []TMDBGenre `json:"genres"`           // Added genres
}

type TMDBMovieDetails struct {
	ID          int         `json:"id"`
	Title       string      `json:"title"`
	Overview    string      `json:"overview"`
	PosterPath  string      `json:"poster_path"`
	ReleaseDate string      `json:"release_date"`
	VoteAverage float64     `json:"vote_average"`
	Runtime     int         `json:"runtime"`
	Genres      []TMDBGenre `json:"genres"` // Added genres
}

func searchTMDB(query string) ([]TMDBResult, error) {
	apiKey := os.Getenv("TMDB_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TMDB_API_KEY not set")
	}

	searchURL := fmt.Sprintf("%s/search/multi?api_key=%s&query=%s", tmdbBaseURL, apiKey, url.QueryEscape(query))
	resp, err := http.Get(searchURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var searchResult TMDBMultiSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return nil, err
	}

	// Return top 5 results, filtered by media type (exclude 'person')
	var filtered []TMDBResult
	for _, res := range searchResult.Results {
		if res.MediaType == "movie" || res.MediaType == "tv" {
			filtered = append(filtered, res)
		}
		if len(filtered) >= 5 {
			break
		}
	}

	return filtered, nil
}

func getTMDBTVDetails(id int) (*TMDBTVDetails, error) {
	apiKey := os.Getenv("TMDB_API_KEY")
	detailsURL := fmt.Sprintf("%s/tv/%d?api_key=%s", tmdbBaseURL, id, apiKey)
	resp, err := http.Get(detailsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var details TMDBTVDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, err
	}
	return &details, nil
}

func getTMDBMovieDetails(id int) (*TMDBMovieDetails, error) {
	apiKey := os.Getenv("TMDB_API_KEY")
	detailsURL := fmt.Sprintf("%s/movie/%d?api_key=%s", tmdbBaseURL, id, apiKey)
	resp, err := http.Get(detailsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var details TMDBMovieDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, err
	}
	return &details, nil
}
