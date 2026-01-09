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

// URLs for NPM-Neon Panel (Agent Account)
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
// GLOBAL RAM STORAGE (NPM-Neon Specific)
// =========================================================
var (
	activeClient *Client    
	clientMutex  sync.Mutex 
)

// GetSession: Returns existing client from RAM or creates new
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
// LOGIN LOGIC (Cookie Based)
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

	// Captcha Logic
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

	// Login POST
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

	u, _ := url.Parse(BaseURL)
	if len(c.HTTPClient.Jar.Cookies(u)) == 0 {
		return errors.New("login failed: no cookies received")
	}
	fmt.Println("[NPM-Neon] Login Successful! Session Saved to RAM.")
	return nil
}

// ---------------------- SMS CLEANING LOGIC (TODAY ONLY) ----------------------

func (c *Client) GetSMSLogs() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	// Auto Re-Login Loop
	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			if i == 0 {
				c.HTTPClient.Jar, _ = cookiejar.New(nil) // Reset cookies
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

		// CHECK: Session Expiry (HTML)
		if bytes.Contains(body, []byte("<!DOCTYPE html>")) || bytes.Contains(body, []byte("<html")) {
			fmt.Println("[NPM-Neon] Session Expired (HTML). Re-logging...")
			c.HTTPClient.Jar, _ = cookiejar.New(nil) // Clear invalid cookies
			continue // Retry
		}

		cleanedJSON, err := cleanSMSData(body)
		if err != nil {
			if i == 0 { continue }
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

	// Raw Neon Agent: [Date(0), Country(1), Number(2), Service(3), User(4), Message(5), Cost(6), Status(7)]
	// Target Format: [Date, Country, Number, Service, Msg, Currency, Cost, Status]

	for _, row := range apiResp.AAData {
		if len(row) > 7 {
			// Message Cleanup
			msg, _ := row[5].(string)
			msg = html.UnescapeString(msg)
			msg = strings.ReplaceAll(msg, "<#>", "")
			msg = strings.ReplaceAll(msg, "null", "")
			msg = strings.TrimSpace(msg)

			newRow := []interface{}{
				row[0], // Date
				row[1], // Country
				row[2], // Number
				row[3], // Service
				msg,    // Full Message
				"$",    // Currency (Usually implicit in Agent panel, hardcoded $)
				row[6], // Cost
				row[7], // Status
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
				c.HTTPClient.Jar, _ = cookiejar.New(nil)
				continue
			}
			return nil, err
		}

		// DATE: 1st Jan 2026 to Today
		fdate1 := "2026-01-01 00:00:00"
		fdate2 := time.Now().Format("2006-01-02") + " 23:59:59"

		params := url.Values{}
		// If API supports date filtering (usually Agent panels do)
		params.Set("fdate1", fdate1) 
		params.Set("fdate2", fdate2)

		params.Set("frange", "")
		params.Set("fclient", "")
		params.Set("sEcho", "2")
		params.Set("iColumns", "8")
		params.Set("sColumns", ",,,,,,,")
		params.Set("iDisplayStart", "0")
		params.Set("iDisplayLength", "-1") // ALL Records
		
		for j := 0; j < 8; j++ {
			idx := strconv.Itoa(j)
			params.Set("mDataProp_"+idx, idx)
			params.Set("sSearch_"+idx, "")
			params.Set("bRegex_"+idx, "false")
			params.Set("bSearchable_"+idx, "true")
			
			sortable := "true"
			if j == 0 || j == 7 { sortable = "false" }
			params.Set("bSortable_"+idx, sortable)
		}

		params.Set("sSearch", "")
		params.Set("bRegex", "false")
		params.Set("iSortingCols", "1")
		params.Set("iSortCol_0", "0")
		params.Set("sSortDir_0", "asc")

		finalURL := NumberApiURL + "?" + params.Encode()

		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Referer", BaseURL+"/ints/agent/MySMSNumbers")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE html>")) || bytes.Contains(body, []byte("<html")) {
			fmt.Println("[NPM-Neon] Session Expired (Numbers). Re-logging...")
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		cleanedJSON, err := cleanNumberData(body)
		if err != nil {
			if i == 0 { continue }
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
		// Agent Raw: [Checkbox(0), Range(1), Prefix(2), Number(3), PriceHTML(4), Action(5), Empty(6), Stats(7)]
		if len(row) > 7 {
			rangeName := row[1]
			prefix := row[2]
			number := row[3]
			
			rawPrice, _ := row[4].(string)
			billingType := "Weekly"
			if strings.Contains(strings.ToLower(rawPrice), "monthly") {
				billingType = "Monthly"
			}

			// ================= FIX IS HERE =================
			currency := "$"
			if strings.Contains(rawPrice, "€") {
				currency = "€"
			} else if strings.Contains(rawPrice, "£") {
				currency = "£"
			}
			// ===============================================

			priceVal := "0"
			matches := rePrice.FindAllString(rawPrice, -1)
			if len(matches) > 0 {
				priceVal = matches[len(matches)-1]
			}
			priceStr := currency + " " + priceVal

			stats := row[7]

			// Unified Format: [Country, Prefix, Number, Type, Price, Stats]
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