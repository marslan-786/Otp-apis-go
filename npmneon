package npmneon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// URLs for NPM-Neon Panel (IP: 144.217.66.209)
const (
	BaseURL      = "http://144.217.66.209"
	LoginURL     = BaseURL + "/ints/login"
	SigninURL    = BaseURL + "/ints/signin"
	SMSApiURL    = BaseURL + "/ints/agent/res/data_smscdr.php"
	NumberApiURL = BaseURL + "/ints/agent/res/data_smsnumbers.php" // Note: This is inventory list, not stats
)

type Client struct {
	HTTPClient *http.Client
	Mutex      sync.Mutex
	// Is panel me sesskey URL me nahi chahiye hoti, sirf cookie kafi hai
}

// JSON Response Wrapper to intercept and clean data
type ApiResponse struct {
	SEcho                interface{}     `json:"sEcho"`
	ITotalRecords        interface{}     `json:"iTotalRecords"`
	ITotalDisplayRecords interface{}     `json:"iTotalDisplayRecords"`
	AAData               [][]interface{} `json:"aaData"`
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

// Session check (Cookie Based)
func (c *Client) ensureSession() error {
	u, _ := url.Parse(BaseURL)
	cookies := c.HTTPClient.Jar.Cookies(u)
	if len(cookies) > 0 {
		// Assume session is valid if cookies exist
		return nil
	}
	fmt.Println("[NPM-Neon] No cookies found. Logging in...")
	return c.performLogin()
}

func (c *Client) performLogin() error {
	fmt.Println("[NPM-Neon] >> Step 1: Login Page")
	
	req, _ := http.NewRequest("GET", LoginURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// Captcha: What is 9 + 7 = ?
	fmt.Println("[NPM-Neon] >> Step 2: Solving Captcha")
	re := regexp.MustCompile(`What is (\d+) \+ (\d+) = \?`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) < 3 {
		return errors.New("captcha math failed")
	}
	num1, _ := strconv.Atoi(matches[1])
	num2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(num1 + num2)
	fmt.Printf("[NPM-Neon] Captcha Solved: %s\n", captchaAns)

	// Step 3: Login POST
	data := url.Values{}
	data.Set("username", "Kami526")
	data.Set("password", "Kami526")
	data.Set("capt", captchaAns)

	loginReq, _ := http.NewRequest("POST", SigninURL, bytes.NewBufferString(data.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
	loginReq.Header.Set("Referer", LoginURL)

	resp, err = c.HTTPClient.Do(loginReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check if login successful (by checking redirect or cookie)
	u, _ := url.Parse(BaseURL)
	if len(c.HTTPClient.Jar.Cookies(u)) == 0 {
		return errors.New("login failed: no cookies received")
	}
	fmt.Println("[NPM-Neon] Login Successful!")
	return nil
}

// ---------------------- SMS CLEANING LOGIC ----------------------

func (c *Client) GetSMSLogs() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			return nil, err
		}

		// Dates logic matches previous panels
		now := time.Now()
		// Neon logs show data exists for older dates, keeping window wide or specific?
		// User used: 2025-12-24 to 2025-12-30 in logs. Let's use standard today logic 
		// OR modify if you want wide range. Defaulting to Today.
		dateStr := now.Format("2006-01-02")
		
		params := url.Values{}
		params.Set("fdate1", dateStr+" 00:00:00")
		params.Set("fdate2", dateStr+" 23:59:59")
		// Params from your log
		params.Set("frange", "")
		params.Set("fclient", "")
		params.Set("fg", "0") 
		params.Set("sEcho", "1")
		params.Set("iDisplayLength", "100") 
		params.Set("iSortingCols", "1")
		params.Set("sSortDir_0", "desc")

		finalURL := SMSApiURL + "?" + params.Encode()
		fmt.Println("[NPM-Neon] Fetching SMS...")

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
			// Session expired logic
			c.HTTPClient.Jar, _ = cookiejar.New(nil) // Clear cookies
			continue
		}

		// --- CLEANING START ---
		cleanedJSON, err := cleanSMSData(body)
		if err != nil {
			return nil, err // If cleaning fails, return error
		}
		// --- CLEANING END ---

		return cleanedJSON, nil
	}
	return nil, errors.New("failed after retry")
}

func cleanSMSData(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil // If not JSON (maybe error msg), return raw
	}

	// Loop through rows
	for i, row := range apiResp.AAData {
		if len(row) > 5 {
			// Index 5 is the Message. 
			// Raw: "Akun... &lt;#&gt;... nCode..."
			msg, ok := row[5].(string)
			if ok {
				// 1. Unescape HTML ( &lt; -> < )
				cleanMsg := html.UnescapeString(msg)
				// 2. Remove Specific Trash
				cleanMsg = strings.ReplaceAll(cleanMsg, "n", " ") // Often 'n' appears as newline artifact
				cleanMsg = strings.ReplaceAll(cleanMsg, "<#>", "") 
				
				// Update the row
				apiResp.AAData[i][5] = cleanMsg
			}
		}
	}

	return json.Marshal(apiResp)
}

// ---------------------- NUMBERS CLEANING LOGIC ----------------------

func (c *Client) GetNumberStats() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			return nil, err
		}

		// Numbers API params
		params := url.Values{}
		params.Set("sEcho", "1")
		params.Set("iDisplayLength", "-1") // Fetch All
		params.Set("iSortingCols", "1")
		params.Set("sSortDir_0", "asc")

		finalURL := NumberApiURL + "?" + params.Encode()
		fmt.Println("[NPM-Neon] Fetching Numbers...")

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
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		// --- CLEANING START ---
		cleanedJSON, err := cleanNumberData(body)
		if err != nil {
			return nil, err
		}
		// --- CLEANING END ---

		return cleanedJSON, nil
	}
	return nil, errors.New("failed after retry")
}

func cleanNumberData(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}

	// New Clean Data Array to match D-Group format: [Number, Count, Currency, Price, 0]
	// Raw Neon format: [Checkbox, Country, Prefix, Number, PriceHTML, Action, Empty, Stats]
	var cleanedRows [][]interface{}

	rePrice := regexp.MustCompile(`[\d\.]+`) // Extracts numbers like 0.01

	for _, row := range apiResp.AAData {
		if len(row) > 4 {
			// Extract Number (Index 3)
			number := row[3]

			// Extract Price (Index 4): "Monthly30<br /><b>\u20ac 0.01</b>"
			rawPrice, _ := row[4].(string)
			
			// Detect Currency
			currency := "$"
			if strings.Contains(rawPrice, "\u20ac") || strings.Contains(rawPrice, "€") {
				currency = "€"
			} else if strings.Contains(rawPrice, "£") {
				currency = "£"
			}

			// Extract numerical price
			priceMatches := rePrice.FindAllString(rawPrice, -1)
			price := 0.0
			if len(priceMatches) > 0 {
				// usually the last number is the price (e.g. "30" then "0.01")
				lastVal := priceMatches[len(priceMatches)-1]
				p, _ := strconv.ParseFloat(lastVal, 64)
				price = p
			}

			// Construct D-Group Style Row
			// [Number, Count(1), Currency, Price, 0]
			newRow := []interface{}{
				number,
				"1", // D-Group usually has count here. Since this is inventory, assume 1.
				currency,
				price,
				0,
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}

	// Update the response with new cleaned structure
	apiResp.AAData = cleanedRows
	// Update records count if filtered
	apiResp.ITotalRecords = len(cleanedRows)
	apiResp.ITotalDisplayRecords = len(cleanedRows)

	return json.Marshal(apiResp)
}
