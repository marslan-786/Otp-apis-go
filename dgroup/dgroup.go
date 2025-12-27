package dgroup

import (
	"bytes"
	"errors"
	"fmt" // <--- Printing k liye add kia
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
			Timeout: 60 * time.Second, // Timeout barha dia ta k jaldi fail na ho
		},
	}
}

// ensureSession checks login status
func (c *Client) ensureSession() error {
	if c.SessKey != "" {
		fmt.Println("[LOG] Session Key pehle se मौजूद hai: ", c.SessKey)
		return nil
	}
	fmt.Println("[LOG] Session Key nahi mili, Login start kar raha hun...")
	return c.performLogin()
}

func (c *Client) performLogin() error {
	fmt.Println("------------------------------------------------")
	fmt.Println("[STEP 1] Login Page Fetch kar raha hun...")
	
	req, _ := http.NewRequest("GET", LoginURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		fmt.Println("[ERROR] Login Page load nahi hua: ", err)
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	fmt.Println("[STEP 2] Captcha dhoond raha hun...")
	// Solve 7 + 4 = ?
	re := regexp.MustCompile(`What is (\d+) \+ (\d+) = \?`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) < 3 {
		fmt.Println("[ERROR] Captcha pattern match nahi hua HTML me.")
		// Debug k liye thora sa HTML print karwao
		fmt.Println("[DEBUG HTML START]", bodyString[:500], "[DEBUG HTML END]")
		return errors.New("captcha math failed")
	}
	num1, _ := strconv.Atoi(matches[1])
	num2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(num1 + num2)
	fmt.Printf("[LOG] Captcha Solved: %d + %d = %s\n", num1, num2, captchaAns)

	// 3. Post Login
	fmt.Println("[STEP 3] Login Data POST kar raha hun...")
	data := url.Values{}
	data.Set("username", "Kami526")
	data.Set("password", "Kamran5.")
	data.Set("capt", captchaAns)

	// PostForm k bajaye Do use karengy ta k Header laga saken
	loginReq, _ := http.NewRequest("POST", SigninURL, bytes.NewBufferString(data.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")

	resp, err = c.HTTPClient.Do(loginReq)
	if err != nil {
		fmt.Println("[ERROR] Login POST fail ho gaya: ", err)
		return err
	}
	defer resp.Body.Close()
	fmt.Println("[LOG] Login Request Status Code: ", resp.StatusCode)

	// 4. Get SessKey from Reports Page
	fmt.Println("[STEP 4] Reports Page se SessKey utha raha hun...")
	reportReq, _ := http.NewRequest("GET", ReportsPage, nil)
	reportReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")

	resp, err = c.HTTPClient.Do(reportReq)
	if err != nil {
		fmt.Println("[ERROR] Reports Page load nahi hua: ", err)
		return err
	}
	defer resp.Body.Close()
	reportBody, _ := io.ReadAll(resp.Body)
	
	// Regex for sesskey
	sessRe := regexp.MustCompile(`sesskey=([a-zA-Z0-9%=]+)`)
	sessMatch := sessRe.FindStringSubmatch(string(reportBody))
	
	if len(sessMatch) > 1 {
		c.SessKey = sessMatch[1]
		fmt.Println("[SUCCESS] SessKey mil gayi: ", c.SessKey)
	} else {
		fmt.Println("[ERROR] SessKey HTML me nahi mili.")
		return errors.New("sesskey not found")
	}

	return nil
}

func (c *Client) GetSMSLogs() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	fmt.Println(">> SMS API Hit Hui")

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

		finalURL := SMSApiURL + "?" + params.Encode()
		fmt.Println("[STEP 5] Final SMS API Call: ", finalURL)

		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		// Cookie Jar automatically handles cookies

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			fmt.Println("[ERROR] API Request Fail: ", err)
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		fmt.Printf("[LOG] API Response Length: %d bytes\n", len(body))

		// Check if HTML (Session Expired)
		if bytes.Contains(body, []byte("<!DOCTYPE html>")) {
			fmt.Println("[WARNING] Session Expire ho gaya (HTML detected). Re-login kar raha hun...")
			c.SessKey = "" // Reset and retry
			continue
		}
		
		fmt.Println("[SUCCESS] Data JSON me mil gaya!")
		return body, nil
	}
	return nil, errors.New("failed after retry")
}

func (c *Client) GetNumberStats() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	fmt.Println(">> Number Stats API Hit Hui")

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

		finalURL := NumberApiURL + "?" + params.Encode()
		fmt.Println("[STEP 5] Final Number API Call: ", finalURL)

		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			fmt.Println("[ERROR] Number API Request Fail: ", err)
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE html>")) {
			fmt.Println("[WARNING] Session Expire (Numbers). Retrying...")
			c.SessKey = ""
			continue
		}
		fmt.Println("[SUCCESS] Number Data Retrieved.")
		return body, nil
	}
	return nil, errors.New("failed after retry")
}
