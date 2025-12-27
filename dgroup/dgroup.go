package dgroup // <--- Ye Naam Folder k naam jesa hona chahiye

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// URLs
const (
	BaseURL      = "http://139.99.63.204"
	LoginURL     = BaseURL + "/ints/login"
	SigninURL    = BaseURL + "/ints/signin"
	ReportsPage  = BaseURL + "/ints/agent/SMSCDRReports"
	SMSApiURL    = BaseURL + "/ints/agent/res/data_smscdr.php"
	NumberApiURL = BaseURL + "/ints/agent/res/data_smsnumberstats.php"
)

type Client struct {
	HTTPClient *http.Client
	SessKey    string
	Mutex      sync.Mutex
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		HTTPClient: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) ensureSession() error {
	if c.SessKey != "" {
		return nil
	}
	return c.performLogin()
}

func (c *Client) performLogin() error {
	// 1. Get Login Page for Captcha
	resp, err := c.HTTPClient.Get(LoginURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// 2. Solve 7 + 4 = ?
	re := regexp.MustCompile(`What is (\d+) \+ (\d+) = \?`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) < 3 {
		return errors.New("captcha math failed")
	}
	num1, _ := strconv.Atoi(matches[1])
	num2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(num1 + num2)

	// 3. Post Login
	data := url.Values{}
	data.Set("username", "Kami526")
	data.Set("password", "Kamran5.")
	data.Set("capt", captchaAns)

	resp, err = c.HTTPClient.PostForm(SigninURL, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 4. Get SessKey from Reports Page
	resp, err = c.HTTPClient.Get(ReportsPage)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reportBody, _ := io.ReadAll(resp.Body)
	
	// Regex for sesskey
	sessRe := regexp.MustCompile(`sesskey=([a-zA-Z0-9%=]+)`)
	sessMatch := sessRe.FindStringSubmatch(string(reportBody))
	
	if len(sessMatch) > 1 {
		c.SessKey = sessMatch[1]
	} else {
		return errors.New("sesskey not found")
	}

	return nil
}

func (c *Client) GetSMSLogs() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			return nil, err
		}

		now := time.Now()
		dateStr := now.Format("2006-01-02")
		
		params := url.Values{}
		params.Set("fdate1", dateStr+" 00:00:00")
		params.Set("fdate2", dateStr+" 23:59:59")
		params.Set("sesskey", c.SessKey)
		params.Set("sEcho", "3")
		params.Set("iDisplayLength", "100")
		params.Set("iSortingCols", "1")
		params.Set("sSortDir_0", "desc")

		req, _ := http.NewRequest("GET", SMSApiURL+"?"+params.Encode(), nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		// Check if HTML (Session Expired)
		if bytes.Contains(body, []byte("<!DOCTYPE html>")) {
			c.SessKey = "" // Reset and retry
			continue
		}
		return body, nil
	}
	return nil, errors.New("failed after retry")
}

func (c *Client) GetNumberStats() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			return nil, err
		}

		now := time.Now()
		dateStr := now.Format("2006-01-02")
		
		params := url.Values{}
		params.Set("fdate1", dateStr+" 00:00:00")
		params.Set("fdate2", dateStr+" 23:59:59")
		params.Set("sEcho", "2")
		params.Set("iDisplayLength", "-1")

		req, _ := http.NewRequest("GET", NumberApiURL+"?"+params.Encode(), nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE html>")) {
			c.SessKey = ""
			continue
		}
		return body, nil
	}
	return nil, errors.New("failed after retry")
}
