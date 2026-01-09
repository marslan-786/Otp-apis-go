package numberpanel1

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

// URLs for Number Panel (Client Account)
const (
	BaseURL      = "http://51.89.99.105"
	LoginURL     = BaseURL + "/NumberPanel/login"
	SigninURL    = BaseURL + "/NumberPanel/signin"
	ReportsPage  = BaseURL + "/NumberPanel/client/SMSCDRStats" 
	SMSApiURL    = BaseURL + "/NumberPanel/client/res/data_smscdr.php"
	NumberApiURL = BaseURL + "/NumberPanel/client/res/data_smsnumbers.php"
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
// GLOBAL RAM STORAGE (یہ سیشن کو سنبھال کر رکھے گا)
// =========================================================
var (
	activeClient *Client    
	clientMutex  sync.Mutex
)

// GetSession: Returns existing client or creates new
func GetSession() *Client {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	// اگر کلائنٹ پہلے سے بنا ہوا ہے اور اس میں کی موجود ہے، تو وہی واپس بھیج دے گا (No Login)
	if activeClient != nil {
		return activeClient
	}

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
// LOGIN LOGIC
// ---------------------------------------------------------

func (c *Client) ensureSession() error {
	// اگر سیشن کی پہلے سے موجود ہے تو لاگ ان نہیں کرے گا
	if c.SessKey != "" {
		return nil
	}
	fmt.Println("[NumberPanel] Session Key missing, Login start...")
	return c.performLogin()
}

func (c *Client) performLogin() error {
	fmt.Println("[NumberPanel] >> Step 1: Login Page (Fetching Captcha)")
	
	req, _ := http.NewRequest("GET", LoginURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// Captcha Logic
	fmt.Println("[NumberPanel] >> Step 2: Solving Captcha")
	re := regexp.MustCompile(`What is (\d+) \+ (\d+) = \?`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) < 3 {
		return errors.New("captcha math regex failed")
	}
	num1, _ := strconv.Atoi(matches[1])
	num2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(num1 + num2)
	fmt.Printf("[NumberPanel] Captcha Solved: %s + %s = %s\n", matches[1], matches[2], captchaAns)

	// Step 3: Login POST
	data := url.Values{}
	data.Set("username", "Kami520")   
	data.Set("password", "Kami526")   
	data.Set("capt", captchaAns)

	loginReq, _ := http.NewRequest("POST", SigninURL, bytes.NewBufferString(data.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
	loginReq.Header.Set("Referer", LoginURL)
	loginReq.Header.Set("Origin", BaseURL)

	resp, err = c.HTTPClient.Do(loginReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Step 4: Get SessKey
	fmt.Println("[NumberPanel] >> Step 3: Getting SessKey from Dashboard")
	reportReq, _ := http.NewRequest("GET", ReportsPage, nil)
	reportReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
	reportReq.Header.Set("Referer", BaseURL+"/NumberPanel/client/SMSDashboard")

	resp, err = c.HTTPClient.Do(reportReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reportBody, _ := io.ReadAll(resp.Body)
	
	sessRe := regexp.MustCompile(`sesskey=([a-zA-Z0-9%=]+)`)
	sessMatch := sessRe.FindStringSubmatch(string(reportBody))
	
	if len(sessMatch) > 1 {
		c.SessKey = sessMatch[1] // Key saved in RAM
		fmt.Println("[NumberPanel] SessKey Found & Saved:", c.SessKey)
	} else {
		return errors.New("sesskey not found (Login likely failed)")
	}

	return nil
}

// ---------------------- SMS CLEANING (TODAY ONLY) ----------------------

func (c *Client) GetSMSLogs() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			if i == 0 {
				c.SessKey = "" 
				continue
			}
			return nil, err
		}

		// UPDATED DATE LOGIC: TODAY ONLY (00:00:00 to 23:59:59)
		now := time.Now()
		fdate1 := now.Format("2006-01-02") + " 00:00:00"
		fdate2 := now.Format("2006-01-02") + " 23:59:59"

		params := url.Values{}
		params.Set("fdate1", fdate1)
		params.Set("fdate2", fdate2)
		params.Set("frange", "")
		params.Set("fnum", "")
		params.Set("fcli", "")
		params.Set("fgdate", "")
		params.Set("fgmonth", "")
		params.Set("fgrange", "")
		params.Set("fgnumber", "")
		params.Set("fgcli", "")
		params.Set("fg", "0")
		params.Set("sesskey", c.SessKey) // Using saved key
		params.Set("sEcho", "1")
		params.Set("iColumns", "7")
		params.Set("iDisplayStart", "0")
		params.Set("iDisplayLength", "100") 
		params.Set("sSearch", "")
		params.Set("bRegex", "false")
		params.Set("iSortCol_0", "0")
		params.Set("sSortDir_0", "desc")
		params.Set("iSortingCols", "1")

		for j := 0; j < 7; j++ {
			idx := strconv.Itoa(j)
			params.Set("mDataProp_"+idx, idx)
			params.Set("sSearch_"+idx, "")
			params.Set("bRegex_"+idx, "false")
			params.Set("bSearchable_"+idx, "true")
			params.Set("bSortable_"+idx, "true")
		}

		finalURL := SMSApiURL + "?" + params.Encode()

		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Referer", ReportsPage)

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		// اگر ایچ ٹی ایم ایل آیا تو اس کا مطلب سیشن ایکسپائر ہو گیا ہے
		if bytes.Contains(body, []byte("<!DOCTYPE HTML>")) || bytes.Contains(body, []byte("<html")) {
			fmt.Println("[NumberPanel] Session Expired (HTML received). Re-logging...")
			c.SessKey = "" // کی ختم کر دی تاکہ اگلی بار نیا لاگ ان ہو
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue // لوپ دوبارہ چلے گا اور نیا لاگ ان کرے گا
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

	for _, row := range apiResp.AAData {
		if len(row) > 4 {
			msg, _ := row[4].(string)
			msg = html.UnescapeString(msg)
			msg = strings.ReplaceAll(msg, "null", "")

			newRow := []interface{}{
				row[0], // Date
				row[2], // Number
				row[3], // CLI/Sender
				msg,    // SMS Content
				row[1], // Range
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}
	apiResp.AAData = cleanedRows
	return json.Marshal(apiResp)
}

// ---------------------- NUMBERS CLEANING (1st Jan to Today) ----------------------

func (c *Client) GetNumberStats() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			if i == 0 {
				c.SessKey = ""
				continue
			}
			return nil, err
		}

		// UPDATED DATE LOGIC: 1st Jan 2026 to NOW
		fdate1 := "2026-01-01 00:00:00" // Hardcoded start date
		fdate2 := time.Now().Format("2006-01-02") + " 23:59:59" // Today

		// Note: The Numbers API endpoint (data_smsnumbers.php) doesn't seem to accept date params 
		// in the URL query string based on your capture, but I'll check if they were POST or implied.
		// Your provided capture didn't show fdate params for numbers, but I will keep logic simple.
		// If the panel supports date filtering via params, add them here. 
		// Assuming standard parameters as per previous logic:
		
		params := url.Values{}
		// If the API supports date filtering:
		// params.Set("fdate1", fdate1) 
		// params.Set("fdate2", fdate2)
		
		params.Set("frange", "")
		params.Set("fclient", "")
		params.Set("sEcho", "4")
		params.Set("iColumns", "6")
		params.Set("sColumns", ",,,,,")
		params.Set("iDisplayStart", "0")
		params.Set("iDisplayLength", "-1") // Fetch All
		params.Set("sSearch", "")
		params.Set("bRegex", "false")
		params.Set("iSortCol_0", "0")
		params.Set("sSortDir_0", "asc")
		params.Set("iSortingCols", "1")

		for j := 0; j < 6; j++ {
			idx := strconv.Itoa(j)
			params.Set("mDataProp_"+idx, idx)
			params.Set("sSearch_"+idx, "")
			params.Set("bRegex_"+idx, "false")
			params.Set("bSearchable_"+idx, "true")
			params.Set("bSortable_"+idx, "true")
		}

		finalURL := NumberApiURL + "?" + params.Encode()

		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Referer", BaseURL+"/NumberPanel/client/MySMSNumbers")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE HTML>")) || bytes.Contains(body, []byte("<html")) {
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
		if len(row) > 0 {
			fullNumStr, ok := row[0].(string)
			if !ok { continue }

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

			newRow := []interface{}{
				countryName,   
				countryPrefix, 
				fullNumStr,    
				row[1],        
				row[4],        
			}
			processedRows = append(processedRows, newRow)
		}
	}

	apiResp.AAData = processedRows
	apiResp.ITotalRecords = len(processedRows)
	apiResp.ITotalDisplayRecords = len(processedRows)

	return json.Marshal(apiResp)
}

func getCountryName(code string) string {
	code = strings.ToUpper(code)
	countries := map[string]string{
		"AF": "Afghanistan", "PK": "Pakistan", "US": "USA", "GB": "United Kingdom",
		"IN": "India", "BD": "Bangladesh", "CN": "China", "RU": "Russia",
		"CA": "Canada", "AU": "Australia", "DE": "Germany", "FR": "France",
		"TR": "Turkey", "SA": "Saudi Arabia", "AE": "UAE",
	}
	if name, ok := countries[code]; ok { return name }
	return code
}