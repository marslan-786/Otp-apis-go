package dgroup

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

	"github.com/nyaruka/phonenumbers"
)

// URLs (D-Group Client Panel)
const (
	BaseURL      = "http://139.99.63.204"
	LoginURL     = BaseURL + "/ints/login"
	SigninURL    = BaseURL + "/ints/signin"
	ReportsPage  = BaseURL + "/ints/client/SMSCDRStats" 
	SMSApiURL    = BaseURL + "/ints/client/res/data_smscdr.php"
	NumberApiURL = BaseURL + "/ints/client/res/data_smsnumbers.php"
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
// GLOBAL RAM STORAGE (Isolated for D-Group)
// =========================================================
var (
	activeClient *Client
	clientMutex  sync.Mutex
)

func GetSession() *Client {
	clientMutex.Lock()
	defer clientMutex.Unlock()
	
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
// LOGIN LOGIC (Client Account: Kami527)
// ---------------------------------------------------------

func (c *Client) ensureSession() error {
	if c.SessKey != "" {
		return nil
	}
	fmt.Println("[D-Group] Session Key missing, Login start...")
	return c.performLogin()
}

func (c *Client) performLogin() error {
	fmt.Println("[D-Group] >> Step 1: Login Page")
	
	req, _ := http.NewRequest("GET", LoginURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// Captcha Logic
	fmt.Println("[D-Group] >> Step 2: Solving Captcha")
	re := regexp.MustCompile(`What is (\d+) \+ (\d+) = \?`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) < 3 {
		return errors.New("captcha math failed")
	}
	n1, _ := strconv.Atoi(matches[1])
	n2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(n1 + n2)

	// Login POST
	data := url.Values{}
	data.Set("username", "Kami527") 
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

	// Get SessKey
	fmt.Println("[D-Group] >> Step 3: Getting SessKey")
	reportReq, _ := http.NewRequest("GET", ReportsPage, nil)
	reportReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
	reportReq.Header.Set("Referer", BaseURL+"/ints/client/SMSDashboard")

	resp, err = c.HTTPClient.Do(reportReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rBody, _ := io.ReadAll(resp.Body)
	
	sessRe := regexp.MustCompile(`sesskey=([a-zA-Z0-9%=]+)`)
	sessMatch := sessRe.FindStringSubmatch(string(rBody))
	
	if len(sessMatch) > 1 {
		c.SessKey = sessMatch[1] // Save to RAM
		fmt.Println("[D-Group] SessKey Saved:", c.SessKey)
	} else {
		// Sometimes client panel relies on cookies, but let's warn
		fmt.Println("[D-Group] Warning: SessKey not found, using Cookies only.")
		c.SessKey = "cookie_mode" 
	}

	return nil
}

// ---------------------- SMS LOGIC (TODAY ONLY) ----------------------

func (c *Client) GetSMSLogs() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	// Auto Re-login Loop
	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			if i == 0 {
				c.SessKey = ""
				c.HTTPClient.Jar, _ = cookiejar.New(nil)
				continue
			}
			return nil, err
		}

		// DATE: Today Only (00:00 to 23:59)
		now := time.Now()
		fdate1 := now.Format("2006-01-02") + " 00:00:00"
		fdate2 := now.Format("2006-01-02") + " 23:59:59"

		params := url.Values{}
		params.Set("fdate1", fdate1)
		params.Set("fdate2", fdate2)
		params.Set("frange", "")
		params.Set("fnum", "")
		params.Set("fcli", "")
		params.Set("fg", "0")
		
		if c.SessKey != "cookie_mode" {
			params.Set("sesskey", c.SessKey)
		}
		
		params.Set("sEcho", "1")
		params.Set("iDisplayLength", "100")
		params.Set("sSortDir_0", "desc")
		params.Set("iSortingCols", "1")
		params.Set("iColumns", "7")

		// Dummy cols
		for j := 0; j < 7; j++ {
			idx := strconv.Itoa(j)
			params.Set("mDataProp_"+idx, idx)
			params.Set("bSearchable_"+idx, "true")
		}

		req, _ := http.NewRequest("GET", SMSApiURL+"?"+params.Encode(), nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Referer", ReportsPage)

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		// CHECK: Session Expired (HTML received)
		if bytes.Contains(body, []byte("<!DOCTYPE html>")) || bytes.Contains(body, []byte("<html")) {
			fmt.Println("[D-Group] Session Expired (HTML). Re-logging...")
			c.SessKey = ""
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue // Retry Login
		}

		return cleanDGroupSMS(body)
	}
	return nil, errors.New("failed after retry")
}

func cleanDGroupSMS(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}
	var cleanedRows [][]interface{}

	for _, row := range apiResp.AAData {
		// D-Group Client Raw: [Date, Range, Number, Sender, Message, Currency, Cost]
		if len(row) > 4 {
			msg, _ := row[4].(string)
			msg = html.UnescapeString(msg)
			msg = strings.ReplaceAll(msg, "null", "")

			cost := "0"
			if len(row) > 6 {
				if val, ok := row[6].(string); ok {
					cost = val
				}
			}

			// Format: [Date, Range, Number, Sender, Msg, Cur, Cost]
			newRow := []interface{}{
				row[0], // Date
				row[1], // Range / Country
				row[2], // Number
				row[3], // Service / Sender
				msg,    // Full Message
				row[5], // Currency ($)
				cost,   // Cost
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}
	apiResp.AAData = cleanedRows
	return json.Marshal(apiResp)
}

// ---------------------- NUMBERS LOGIC (Jan 1st to Today) ----------------------

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

		// DATE: 1st Jan 2026 to Today
		fdate1 := "2026-01-01 00:00:00" 
		fdate2 := time.Now().Format("2006-01-02") + " 23:59:59"

		params := url.Values{}
		params.Set("fdate1", fdate1) // If API supports date
		params.Set("fdate2", fdate2)
		params.Set("frange", "")
		params.Set("fclient", "")
		
		if c.SessKey != "cookie_mode" {
			params.Set("sesskey", c.SessKey)
		}

		params.Set("sEcho", "2")
		params.Set("iDisplayLength", "-1") // Fetch All
		params.Set("sSortDir_0", "asc")
		params.Set("iColumns", "6")

		for j := 0; j < 6; j++ {
			idx := strconv.Itoa(j)
			params.Set("mDataProp_"+idx, idx)
			params.Set("bSearchable_"+idx, "true")
		}

		req, _ := http.NewRequest("GET", NumberApiURL+"?"+params.Encode(), nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Referer", BaseURL+"/ints/client/MySMSNumbers")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		// CHECK: Session Expired (Numbers)
		if bytes.Contains(body, []byte("<!DOCTYPE html>")) || bytes.Contains(body, []byte("<html")) {
			fmt.Println("[D-Group] Session Expired (Numbers). Re-logging...")
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
		// Client Raw: [Range(0), Prefix(1), Number(2), Payterm(3), Payout(4), Limits(5)]
		// Note: Prefix column (1) is usually empty or partial. We calculate Country Code manually.
		
		if len(row) > 2 {
			rangeName, _ := row[0].(string)
			fullNumStr, ok := row[2].(string) // Index 2 is Full Number
			if !ok { continue }

			// Clean Number
			fullNumStr = strings.ReplaceAll(fullNumStr, " ", "")
			fullNumStr = strings.ReplaceAll(fullNumStr, "-", "")

			// 1. Get Country Code
			countryCodeStr := ""
			parseNumStr := fullNumStr
			if !strings.HasPrefix(parseNumStr, "+") { parseNumStr = "+" + parseNumStr }

			numObj, err := phonenumbers.Parse(parseNumStr, "")
			if err == nil {
				countryCodeStr = strconv.Itoa(int(numObj.GetCountryCode()))
			} else {
				// Fallback: extract from start if parse fails
				if len(fullNumStr) > 3 { countryCodeStr = fullNumStr[:3] }
			}

			// 2. Clean Price/Stats if needed
			payTerm, _ := row[3].(string)
			payout, _ := row[4].(string)
			limits, _ := row[5].(string) // This usually acts as Stats

			// Unified Structure (Same as MAIT/NumberPanel):
			// [0] Range Name
			// [1] Country Code
			// [2] Full Number
			// [3] Period/Payterm
			// [4] Price/Payout
			// [5] Stats/Limits

			newRow := []interface{}{
				rangeName,      // [0] Main Title
				countryCodeStr, // [1] Subtitle 1 (Code)
				fullNumStr,     // [2] Subtitle 2 (Number)
				payTerm,        // [3] Period
				payout,         // [4] Price
				limits,         // [5] Stats
			}
			processedRows = append(processedRows, newRow)
		}
	}
	apiResp.AAData = processedRows
	apiResp.ITotalRecords = len(processedRows)
	apiResp.ITotalDisplayRecords = len(processedRows)
	return json.Marshal(apiResp)
}