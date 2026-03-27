package main

import (
	"bytes"
	"encoding/json"
	"net/http"
)

const aniListURL = "https://graphql.anilist.co"

type AniListSearchResponse struct {
	Data struct {
		Page struct {
			Media []AniListMedia `json:"media"`
		} `json:"Page"`
	} `json:"data"`
}

type AniListDetailsResponse struct {
	Data struct {
		Media AniListMedia `json:"Media"`
	} `json:"data"`
}

type AniListMedia struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
	} `json:"title"`
	AverageScore int     `json:"averageScore"`
	Episodes     int     `json:"episodes"`
	Duration     int     `json:"duration"` // Added duration
	SeasonYear   int     `json:"seasonYear"`
	Status       string             `json:"status"`
	Description  string             `json:"description"`
	Genres       []string           `json:"genres"` // Added genres
	CoverImage   struct {
		Large string `json:"large"`
	} `json:"coverImage"`
	SiteUrl   string             `json:"siteUrl"`
	Relations AniListRelationBox `json:"relations"` // Added relations
}

type AniListRelationBox struct {
	Edges []AniListRelationEdge `json:"edges"`
}

type AniListRelationEdge struct {
	RelationType string `json:"relationType"`
	Node         struct {
		ID    int    `json:"id"`
		Type  string `json:"type"`
		Title struct {
			English string `json:"english"`
			Romaji  string `json:"romaji"`
		} `json:"title"`
	} `json:"node"`
}

func searchAniList(title string) ([]AniListMedia, error) {
	query := `
	query ($search: String) {
	  Page (perPage: 5) {
		media (search: $search, type: ANIME, sort: POPULARITY_DESC) {
		  id
		  title {
			romaji
			english
		  }
		  seasonYear
		  genres
		}
	  }
	}
	`
	variables := map[string]interface{}{
		"search": title,
	}

	requestBody, err := json.Marshal(map[string]interface{}{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(aniListURL, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result AniListSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data.Page.Media, nil
}

func getAniListDetails(id int) (*AniListMedia, error) {
	query := `
	query ($id: Int) {
	  Media (id: $id, type: ANIME) {
		id
		title {
		  romaji
		  english
		}
		averageScore
		episodes
		duration
		seasonYear
		status
		description
		genres
		coverImage {
		  large
		}
		siteUrl
		relations {
		  edges {
			relationType
			node {
			  id
			  type
			  title {
				english
				romaji
			  }
			}
		  }
		}
	  }
	}
	`
	variables := map[string]interface{}{
		"id": id,
	}

	requestBody, err := json.Marshal(map[string]interface{}{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(aniListURL, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result AniListDetailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result.Data.Media, nil
}
