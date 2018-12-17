package trakt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"

	"github.com/xanderstrike/goplaxt/lib/plex"
	"github.com/xanderstrike/goplaxt/lib/store"
)

const clientId string = "c9a8a36c476dcfe72b46b8be2237e8151486af90dac6b94548c89329f2a190c2"
const clientSecret string = "852aa926322f30d54d98d3693a95dfbf13efcaa7ce18f2fc1ad8b21a8463db51"

func AuthRequest(username, code, refreshToken, grantType string) map[string]interface{} {
	values := map[string]string{
		"code":          code,
		"refresh_token": refreshToken,
		"client_id":     clientId,
		"client_secret": clientSecret,
		"redirect_uri":  fmt.Sprintf("http://localhost:8000/authorize?username=%s", string(username)),
		"grant_type":    grantType,
	}
	jsonValue, _ := json.Marshal(values)

	resp, err := http.Post("https://api.trakt.tv/oauth/token", "application/json", bytes.NewBuffer(jsonValue))
	handleErr(err)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	handleErr(err)

	return result
}

func Handle(pr plex.PlexResponse, user store.User) {
	if pr.Metadata.LibrarySectionType == "show" {
		HandleShow(pr, user.AccessToken)
	} else if pr.Metadata.LibrarySectionType == "movie" {
		HandleMovie(pr, user.AccessToken)
	}
	log.Print("Event logged")
}

func HandleShow(pr plex.PlexResponse, accessToken string) {
	event, progress := getAction(pr)

	scrobbleObject := ShowScrobbleBody{
		Progress: progress,
		Episode:  findEpisode(pr),
	}

	scrobbleJSON, err := json.Marshal(scrobbleObject)
	handleErr(err)

	scrobbleRequest(event, scrobbleJSON, accessToken)
}

func HandleMovie(pr plex.PlexResponse, accessToken string) {
	event, progress := getAction(pr)

	scrobbleObject := MovieScrobbleBody{
		Progress: progress,
		Movie:    findMovie(pr),
	}

	scrobbleJSON, _ := json.Marshal(scrobbleObject)

	scrobbleRequest(event, scrobbleJSON, accessToken)
}

func findEpisode(pr plex.PlexResponse) Episode {
	re := regexp.MustCompile("thetvdb://(\\d*)/(\\d*)/(\\d*)")
	showID := re.FindStringSubmatch(pr.Metadata.Guid)

	log.Print(fmt.Sprintf("Finding show for %s %s %s", showID[1], showID[2], showID[3]))

	url := fmt.Sprintf("https://api.trakt.tv/search/tvdb/%s?type=show", showID[1])

	resp_body := makeRequest(url)

	var showInfo []ShowInfo
	err := json.Unmarshal(resp_body, &showInfo)
	handleErr(err)

	url = fmt.Sprintf("https://api.trakt.tv/shows/%d/seasons?extended=episodes", showInfo[0].Show.Ids.Trakt)

	resp_body = makeRequest(url)
	var seasons []Season
	err = json.Unmarshal(resp_body, &seasons)
	handleErr(err)

	for _, season := range seasons {
		if fmt.Sprintf("%d", season.Number) == showID[2] {
			for _, episode := range season.Episodes {
				if fmt.Sprintf("%d", episode.Number) == showID[3] {
					return episode
				}
			}
		}
	}
	panic("Could not find episode!")
}

func findMovie(pr plex.PlexResponse) Movie {
	log.Print(fmt.Sprintf("Finding movie for %s (%d)", pr.Metadata.Title, pr.Metadata.Year))
	url := fmt.Sprintf("https://api.trakt.tv/search/movie?query=%s", url.PathEscape(pr.Metadata.Title))

	resp_body := makeRequest(url)

	var results []MovieSearchResult

	err := json.Unmarshal(resp_body, &results)
	handleErr(err)

	for _, result := range results {
		if result.Movie.Year == pr.Metadata.Year {
			return result.Movie
		}
	}
	panic("Could not find movie!")
}

func makeRequest(url string) []byte {
	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	handleErr(err)

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("trakt-api-version", "2")
	req.Header.Add("trakt-api-key", clientId)

	resp, err := client.Do(req)
	handleErr(err)

	defer resp.Body.Close()
	resp_body, _ := ioutil.ReadAll(resp.Body)
	return resp_body
}

func scrobbleRequest(action string, body []byte, access_token string) []byte {
	client := &http.Client{}

	url := fmt.Sprintf("https://api.trakt.tv/scrobble/%s", action)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	handleErr(err)

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", access_token))
	req.Header.Add("trakt-api-version", "2")
	req.Header.Add("trakt-api-key", clientId)

	resp, _ := client.Do(req)

	defer resp.Body.Close()
	resp_body, _ := ioutil.ReadAll(resp.Body)
	return resp_body
}

func getAction(pr plex.PlexResponse) (string, int) {
	switch pr.Event {
	case "media.play":
		return "start", 0
	case "media.pause":
		return "stop", 0
	case "media.resume":
		return "start", 0
	case "media.stop":
		return "stop", 0
	case "media.scrobble":
		return "stop", 90
	}
	return "", 0
}

func handleErr(err error) {
	if err != nil {
		panic(err)
	}
}
