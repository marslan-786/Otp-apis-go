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
	NumberApiURL = BaseURL + "/ints/agent/res/data_smsnumbers.php"
)

type Client struct {
	HTTPClient *http.Client
	Mutex      sync.Mutex
}

// JSON Response Wrapper
type ApiResponse struct {
	SEcho                interface{}     `json:"sEcho"`
	ITotalRecords        interface{}     `json:"iTotalRecords"`
	ITotalDisplayRecords interface{}     `json:"iTotalDisplayRecords"`
	AAData               [][]interface{} `json:"aaData"`
}

// =========================================================
// GLOBAL RAM STORAGE (Specific to 'npmneon' package ONLY)
// =========================================================
var (
	activeClient *Client    // یہ ویری ایبل صرف NPM-Neon کا سیشن سنبھالے گا
	clientMutex  sync.Mutex // تھریڈ سیفٹی کے لیے
)

// GetSession: یہ فنکشن ہر بار وہی پرانا کلائنٹ واپس کرے گا (RAM سے)
func GetSession() *Client {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	// اگر کلائنٹ پہلے سے موجود ہے تو وہی واپس کرو
	if activeClient != nil {
		return activeClient
	}

	// اگر پہلی بار کال ہو رہا ہے تو نیا بناؤ اور محفوظ کر لو
	jar, _ := cookiejar.New(nil)
	activeClient = &Client{
		HTTPClient: &http.Client{
			Jar:     jar,
			Timeout: 60 * time.Second,
		},
	}
	return activeClient
}

// ---------------------------------------------------------
// LOGIN LOGIC (Cookie Based + Hardcoded)
// ---------------------------------------------------------

// ensureSession: Check if we have valid cookies
func (c *Client) ensureSession() error {
	u, _ := url.Parse(BaseURL)
	cookies := c.HTTPClient.Jar.Cookies(u)
	if len(cookies) > 0 {
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

	// Step 3: Login POST (HARDCODED CREDENTIALS)
	data := url.Values{}
	data.Set("username", "Kami526") // Hardcoded User
	data.Set("password", "Kami526") // Hardcoded Pass
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

	u, _ := url.Parse(BaseURL)
	if len(c.HTTPClient.Jar.Cookies(u)) == 0 {
		return errors.New("login failed: no cookies received")
	}
	fmt.Println("[NPM-Neon] Login Successful! Session Saved to RAM.")
	return nil
}

// ---------------------- SMS CLEANING LOGIC ----------------------

func (c *Client) GetSMSLogs() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	// Retry Loop: Handles Session Expiry
	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			return nil, err
		}

		// --- DATE LOGIC (1st of Month to Today) ---
		now := time.Now()
		startDate := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		fdate1 := startDate.Format("2006-01-02") + " 00:00:00"
		fdate2 := now.Format("2006-01-02") + " 23:59:59"

		params := url.Values{}
		params.Set("fdate1", fdate1)
		params.Set("fdate2", fdate2)
		
		params.Set("frange", "")
		params.Set("fclient", "")
		params.Set("fg", "0") 
		params.Set("sEcho", "1")
		params.Set("iDisplayLength", "100") 
		params.Set("iSortingCols", "1")
		params.Set("sSortDir_0", "desc")

		finalURL := SMSApiURL + "?" + params.Encode()

		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		// Check Session Expiry
		if bytes.Contains(body, []byte("<!DOCTYPE html>")) {
			fmt.Println("[NPM-Neon] Session Expired. Re-logging...")
			c.HTTPClient.Jar, _ = cookiejar.New(nil) // Reset Cookies
			continue // Auto Retry
		}

		cleanedJSON, err := cleanSMSData(body)
		if err != nil {
			return nil, err 
		}
		return cleanedJSON, nil
	}
	return nil, errors.New("failed after retry")
}

func cleanSMSData(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}

	var cleanedRows [][]interface{}

	// Raw Neon: [Date(0), Country(1), Number(2), Service(3), User(4), Message(5), Cost(6), Status(7), ...]
	// Target: [Date, Country, Number, Service, Message, Currency, Cost, Status] (No User)

	for _, row := range apiResp.AAData {
		if len(row) > 8 {
			// Message Cleanup (Index 5)
			msg, _ := row[5].(string)
			
			// 1. Unescape HTML
			msg = html.UnescapeString(msg)
			
			// 2. Remove Junk
			msg = strings.ReplaceAll(msg, "<#>", "")
			msg = strings.ReplaceAll(msg, "null", "") // Remove literal "null" text
			msg = strings.TrimSpace(msg) // Remove extra spaces/newlines

			// Construct New Row (Skipping Index 4: User)
			newRow := []interface{}{
				row[0], // Date
				row[1], // Country
				row[2], // Number
				row[3], // Service
				msg,    // Message (Moved up)
				row[6], // Currency (Usually $)
				row[7], // Cost
				row[8], // Status
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}

	apiResp.AAData = cleanedRows
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

		params := url.Values{}
		params.Set("sEcho", "1")
		params.Set("iDisplayLength", "-1") // Fetch All
		params.Set("iSortingCols", "1")
		params.Set("sSortDir_0", "asc")

		finalURL := NumberApiURL + "?" + params.Encode()

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
			fmt.Println("[NPM-Neon] Session Expired (Numbers). Re-logging...")
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		cleanedJSON, err := cleanNumberData(body)
		if err != nil {
			return nil, err
		}
		return cleanedJSON, nil
	}
	return nil, errors.New("failed after retry")
}

func cleanNumberData(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}

	var cleanedRows [][]interface{}
	rePrice := regexp.MustCompile(`[\d\.]+`)

	for _, row := range apiResp.AAData {
		// Raw Neon: [Checkbox(0), RangeName(1), Prefix(2), Number(3), PriceHTML(4), Action(5), Empty(6), Stats(7)]
		if len(row) > 7 {
			rangeName := row[1]
			prefix := row[2]
			number := row[3]
			
			// Price & Type Analysis
			rawPrice, _ := row[4].(string)
			billingType := "Weekly" // Default
			if strings.Contains(strings.ToLower(rawPrice), "monthly") {
				billingType = "Monthly"
			}

			currency := "$"
			if strings.Contains(rawPrice, "€") {
				currency = "€"
			} else if strings.Contains(rawPrice, "£") {
				currency = "£"
			}

			priceVal := "0"
			matches := rePrice.FindAllString(rawPrice, -1)
			if len(matches) > 0 {
				priceVal = matches[len(matches)-1]
			}
			priceStr := currency + " " + priceVal

			// Stats (SD/SW counts)
			stats := row[7]

			// Final Format: [Country, Prefix, Number, Type, Price, Stats]
			newRow := []interface{}{
				rangeName,
				prefix,
				number,
				billingType,
				priceStr,
				stats,
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}

	apiResp.AAData = cleanedRows
	apiResp.ITotalRecords = len(cleanedRows)
	apiResp.ITotalDisplayRecords = len(cleanedRows)

	return json.Marshal(apiResp)
}