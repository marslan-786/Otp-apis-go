package numberpanel

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

	// Google's libphonenumber library for Go
	"github.com/nyaruka/phonenumbers"
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

// Wrapper for JSON Response
type ApiResponse struct {
	SEcho                interface{}     `json:"sEcho"`
	ITotalRecords        interface{}     `json:"iTotalRecords"`
	ITotalDisplayRecords interface{}     `json:"iTotalDisplayRecords"`
	AAData               [][]interface{} `json:"aaData"`
}

type Client struct {
	HTTPClient *http.Client
	SessKey    string
	Mutex      sync.Mutex
}

// =========================================================
// GLOBAL RAM STORAGE (Specific to 'numberpanel' package)
// =========================================================
var (
	activeClient *Client    // یہ ویری ایبل صرف NumberPanel کا سیشن سنبھالے گا
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
// LOGIN LOGIC (Hardcoded Credentials)
// ---------------------------------------------------------

func (c *Client) ensureSession() error {
	if c.SessKey != "" {
		return nil
	}
	fmt.Println("[NumberPanel] Session Key missing, Login start...")
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

	// Step 3: Login POST (HARDCODED)
	data := url.Values{}
	data.Set("username", "Kami526")   // Hardcoded Username
	data.Set("password", "Kamran52")  // Hardcoded Password
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
		c.SessKey = sessMatch[1] // SAVED TO RAM OBJECT
		fmt.Println("[NumberPanel] SessKey Found & Saved:", c.SessKey)
	} else {
		return errors.New("sesskey not found")
	}

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

		now := time.Now()
		// Start Date: 1st of Current Month
		startDate := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		
		params := url.Values{}
		params.Set("fdate1", startDate.Format("2006-01-02")+" 00:00:00")
		params.Set("fdate2", now.Format("2006-01-02")+" 23:59:59")
		params.Set("sesskey", c.SessKey)
		params.Set("sEcho", "2")
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
			fmt.Println("[NumberPanel] Session Expired. Re-logging...")
			c.SessKey = "" // Reset Key
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue // Retry loop will login again
		}

		return cleanNumberPanelSMS(body)
	}
	return nil, errors.New("failed after retry")
}

func cleanNumberPanelSMS(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}

	var cleanedRows [][]interface{}

	// Raw: [Date, Range, Number, Service, User(4), Message(5), Currency, Cost, Status]
	for _, row := range apiResp.AAData {
		if len(row) > 5 {
			msg, _ := row[5].(string)
			msg = html.UnescapeString(msg)
			msg = strings.ReplaceAll(msg, "null", "")

			newRow := []interface{}{
				row[0], // Date
				row[1], // Range
				row[2], // Number
				row[3], // Service
				// Skipped Index 4 (User)
				msg,    // Message (Moved Up)
				row[6], // Currency
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

		now := time.Now()
		// Start Date: 1st of Current Month
		startDate := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

		params := url.Values{}
		params.Set("fdate1", startDate.Format("2006-01-02")+" 00:00:00")
		params.Set("fdate2", now.Format("2006-01-02")+" 23:59:59")
		params.Set("sEcho", "1")
		params.Set("iDisplayLength", "-1")

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
			fmt.Println("[NumberPanel] Session Expired (Numbers). Re-logging...")
			c.SessKey = ""
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		return processNumbersWithCountry(body)
	}
	return nil, errors.New("failed after retry")
}

func processNumbersWithCountry(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}

	var processedRows [][]interface{}

	for _, row := range apiResp.AAData {
		// Raw: [Number(0), Count(1), Currency(2), Price(3), Status(4)]
		if len(row) > 0 {
			fullNumStr, ok := row[0].(string)
			if !ok {
				continue
			}

			// Detect Country & Prefix
			countryName := "Unknown"
			countryPrefix := ""

			parseNumStr := fullNumStr
			if !strings.HasPrefix(parseNumStr, "+") {
				parseNumStr = "+" + parseNumStr
			}

			numObj, err := phonenumbers.Parse(parseNumStr, "")
			if err == nil {
				countryPrefix = strconv.Itoa(int(numObj.GetCountryCode()))
				regionCode := phonenumbers.GetRegionCodeForNumber(numObj)
				countryName = getCountryName(regionCode)
			}

			// New Row Construction
			newRow := []interface{}{
				countryName,   // 0
				countryPrefix, // 1
				fullNumStr,    // 2
				row[1],        // 3
				row[2],        // 4
				row[3],        // 5
				row[4],        // 6
			}
			processedRows = append(processedRows, newRow)
		}
	}

	apiResp.AAData = processedRows
	apiResp.ITotalRecords = len(processedRows)
	apiResp.ITotalDisplayRecords = len(processedRows)

	return json.Marshal(apiResp)
}

// Helper to map Region Codes
func getCountryName(code string) string {
	code = strings.ToUpper(code)
	countries := map[string]string{
		"AF": "Afghanistan", "PK": "Pakistan", "US": "USA", "GB": "United Kingdom",
		"IN": "India", "BD": "Bangladesh", "CN": "China", "RU": "Russia",
		"CA": "Canada", "AU": "Australia", "DE": "Germany", "FR": "France",
		// Add more as needed...
	}
	if name, ok := countries[code]; ok {
		return name
	}
	return code
}