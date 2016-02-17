package catlady

import (
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/patrickmn/go-cache"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	userAgent = "Catbot/1 by cattebot"
)

type Image struct {
	url    string
	title  string
	domain string
}

type AuthToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}

type CatLady struct {
	token           AuthToken
	lastTokenTime   time.Time
	catCache        *cache.Cache
	redditUsername  string
	redditPassword  string
	redditAppId     string
	redditAppSecret string
	subreddits      map[string]string
}

func NewCatLady(username string, password string, appid string, appsecret string, subreddits map[string]string, logLevel log.Level) *CatLady {
	log.SetLevel(logLevel)
	c := &CatLady{
		catCache:        cache.New(5*time.Minute, 30*time.Second),
		redditUsername:  username,
		redditPassword:  password,
		redditAppId:     appid,
		redditAppSecret: appsecret,
		subreddits:      subreddits,
	}
	return c
}

func (c *CatLady) getToken() {
	client := &http.Client{}
	data := url.Values{}
	data.Set("grant_type", "password")
	data.Add("username", c.redditUsername)
	data.Add("password", c.redditPassword)
	req, err := http.NewRequest("POST", "https://www.reddit.com/api/v1/access_token", strings.NewReader(data.Encode()))
	if err != nil {
		log.WithError(err).Error("Failed to build Authentication POST")
	}
	req.Header.Add("User-Agent", userAgent)
	req.SetBasicAuth(c.redditAppId, c.redditAppSecret)
	resp, err := client.Do(req)
	if err != nil {
		log.WithError(err).Error("Failed to login to Reddit")
	}
	contents, err := ioutil.ReadAll(resp.Body)
	json.Unmarshal(contents, &c.token)
	c.lastTokenTime = time.Now()
	log.WithFields(log.Fields{
		"AccessToken":   c.token.AccessToken,
		"ExpiresIn":     c.token.ExpiresIn,
		"Scope":         c.token.Scope,
		"tokenType":     c.token.TokenType,
		"lastTokenTime": c.lastTokenTime,
	}).Debug("Got token!")
}

func randInt(min int, max int) int {
	if max <= min {
		return max
	}
	rand.Seed(time.Now().UTC().UnixNano())
	return min + rand.Intn(max-min)
}

func (c *CatLady) getReddit(sub string) RedditResponse {
	log.WithField("subreddit", sub).Debug("Getting Reddit")
	log.WithField("token", c.token).Debug("token value")
	if c.token.ExpiresIn == 0 || time.Since(c.lastTokenTime).Seconds() >= float64(c.token.ExpiresIn) {
		c.getToken()
	}
	client := &http.Client{}
	reqUrl := fmt.Sprintf("https://oauth.reddit.com/r/%s.json", sub)
	log.WithField("url", reqUrl).Debug("Request URL")
	req, err := http.NewRequest("GET", reqUrl, nil)
	if err != nil {
		log.WithError(err).Error("Failed request to reddit")
	}
	req.Header.Add("User-Agent", userAgent)
	req.Header.Add("Authorization", fmt.Sprintf("%s %s", c.token.TokenType, c.token.AccessToken))
	resp, err := client.Do(req)
	if err != nil {
		log.WithError(err).Error("Getting subreddit failed")
	}
	contents, err := ioutil.ReadAll(resp.Body)
	log.WithField("rawjson", string(contents)).Debug("Raw response from reddit")
	var subReddit RedditResponse
	json.Unmarshal(contents, &subReddit)
	return subReddit
}

func checkForImage(url string) bool {
	whitelist := [...]string{"imgur.com", "imgur", "giphy", "flickr", "photobucket", "youtube", "youtu.be", "gif", "gifv", "png", "jpg", "tiff", "webem", "bmp", "flv", "mpg", "mpeg", "avi"}
	for _, thing := range whitelist {
		if strings.Contains(url, thing) {
			log.WithField("url", url).Debug("Found Image")
			return true
		}
	}
	log.WithField("url", url).Debug("Didn't Find Image")
	return false
}

func cleanURL(url string) string {
	if strings.Contains(url, "imgur") {
		log.WithField("url", url).Debug("Found imgur url")
		if url[len(url)-3:] == "gif" {
			url = url + "v"
			log.WithField("url", url).Debug("Converting to gifv")
		}
	}
	return url
}

func (c *CatLady) GetImage(sub string) string {
	var submissions RedditResponse
	if subs, found := c.catCache.Get(sub); !found {
		log.WithFields(log.Fields{
			"cache":     false,
			"subreddit": sub,
		}).Info("Subreddit not found in cache.")
		submissions = c.getReddit(sub)
		c.catCache.Set(sub, submissions, cache.DefaultExpiration)
		log.WithField("subreddit", submissions.Data.Children[0].Data.URL).Debug("Subreddit value")
	} else {
		log.WithFields(log.Fields{
			"cache":     true,
			"subreddit": sub,
		}).Info("Subreddit Found in Cache.")

		submissions = subs.(RedditResponse)

	}
	size := len(submissions.Data.Children)
	count := 0
	noImage := true
	for noImage {
		count += 1
		random := randInt(0, size-1)
		s := submissions.Data.Children[random].Data
		if !s.Over18 {
			if checkForImage(s.URL) {
				noImage = false
				return cleanURL(s.URL)
			}
		} else {
			log.WithField("nsfw", "true").Info("NSFW Link Found.")
		}
		if count >= size {
			log.WithFields(log.Fields{
				"subreddit": sub,
				"size":      size,
			}).Info("I ran out of links")
			return ""
		}
	}
	return ""
}
