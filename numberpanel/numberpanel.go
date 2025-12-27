package numberpanel

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// URLs for Number Panel (IP: 51.89.99.105)
const (
	BaseURL      = "http://51.89.99.105"
	LoginURL     = BaseURL + "/NumberPanel/login"
	SigninURL    = BaseURL + "/NumberPanel/signin"
	ReportsPage  = BaseURL + "/NumberPanel/agent/SMSCDRReports"
	SMSApiURL    = BaseURL + "/NumberPanel/agent/res/data_smscdr.php"
	NumberApiURL = BaseURL + "/NumberPanel/agent/res/data_smsnumberstats.php"
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
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) ensureSession() error {
	if c.SessKey != "" {
		fmt.Println("[NumberPanel] Session Key majood hai.")
		return nil
	}
	fmt.Println("[NumberPanel] Session Key nahi mili, Login start...")
	return c.performLogin()
}

func (c *Client) performLogin() error {
	fmt.Println("[NumberPanel] >> Step 1: Login Page")
	
	req, _ := http.NewRequest("GET", LoginURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// Captcha Logic: What is 6 + 3 = ?
	fmt.Println("[NumberPanel] >> Step 2: Solving Captcha")
	re := regexp.MustCompile(`What is (\d+) \+ (\d+) = \?`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) < 3 {
		return errors.New("captcha math failed")
	}
	num1, _ := strconv.Atoi(matches[1])
	num2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(num1 + num2)
	fmt.Printf("[NumberPanel] Captcha Solved: %s\n", captchaAns)

	// Step 3: Login POST
	data := url.Values{}
	data.Set("username", "Kami526")   // Updated Username
	data.Set("password", "Kamran52")  // Updated Password
	data.Set("capt", captchaAns)

	loginReq, _ := http.NewRequest("POST", SigninURL, bytes.NewBufferString(data.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
	// Add Referer specifically for this panel as some strict panels check it
	loginReq.Header.Set("Referer", LoginURL)

	resp, err = c.HTTPClient.Do(loginReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Step 4: Get SessKey
	fmt.Println("[NumberPanel] >> Step 3: Getting SessKey")
	reportReq, _ := http.NewRequest("GET", ReportsPage, nil)
	reportReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")

	resp, err = c.HTTPClient.Do(reportReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reportBody, _ := io.ReadAll(resp.Body)
	
	sessRe := regexp.MustCompile(`sesskey=([a-zA-Z0-9%=]+)`)
	sessMatch := sessRe.FindStringSubmatch(string(reportBody))
	
	if len(sessMatch) > 1 {
		c.SessKey = sessMatch[1]
		fmt.Println("[NumberPanel] SessKey Found:", c.SessKey)
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
		params.Set("sEcho", "2")
		params.Set("iDisplayLength", "50") // Changed to 50 as per your log
		params.Set("iSortingCols", "1")
		params.Set("sSortDir_0", "desc")

		finalURL := SMSApiURL + "?" + params.Encode()
		fmt.Println("[NumberPanel] Fetching SMS: ", finalURL)

		req, _ := http.NewRequest("GET", finalURL, nil)
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
		params.Set("sEcho", "1")
		params.Set("iDisplayLength", "25")

		finalURL := NumberApiURL + "?" + params.Encode()
		fmt.Println("[NumberPanel] Fetching Numbers: ", finalURL)

		req, _ := http.NewRequest("GET", finalURL, nil)
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
