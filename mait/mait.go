package mait

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

// URLs
const (
	BaseURL      = "http://217.182.195.194"
	LoginURL     = BaseURL + "/ints/login"
	SigninURL    = BaseURL + "/ints/signin"
	ReportsPage  = BaseURL + "/ints/agent/SMSCDRReports"
	SMSApiURL    = BaseURL + "/ints/agent/res/data_smscdr.php"
	NumberApiURL = BaseURL + "/ints/agent/res/data_smsnumbers.php"
)

// Response Struct
type ApiResponse struct {
	SEcho                interface{}     `json:"sEcho"`
	ITotalRecords        interface{}     `json:"iTotalRecords"`
	ITotalDisplayRecords interface{}     `json:"iTotalDisplayRecords"`
	AAData               [][]interface{} `json:"aaData"`
}

type Client struct {
	HTTPClient *http.Client
	Csstr      string // یہ RAM میں محفوظ رہے گی
	Mutex      sync.Mutex
}

// =========================================================
// GLOBAL STORAGE (تاکہ ہر ریکویسٹ پر نیا کلائنٹ نہ بنے)
// =========================================================
var (
	activeClient *Client
	once         sync.Once
)

// GetSession: یہ فنکشن صرف ایک بار کلائنٹ بنائے گا اور اسے ہی استعمال کرے گا
func GetSession() *Client {
	once.Do(func() {
		jar, _ := cookiejar.New(nil)
		activeClient = &Client{
			HTTPClient: &http.Client{
				Jar:     jar,
				Timeout: 60 * time.Second,
			},
		}
	})
	return activeClient
}

// ---------------------------------------------------------
// LOGIN LOGIC
// ---------------------------------------------------------

func (c *Client) ensureSession() error {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	// اگر ہمارے پاس پہلے سے ٹوکن موجود ہے تو لاگ ان مت کرو
	if c.Csstr != "" {
		return nil
	}

	fmt.Println("[Masdar] No active session. Logging in...")
	return c.performLogin()
}

func (c *Client) performLogin() error {
	// Step 1: Login Page (To get Cookies & Captcha)
	req, _ := http.NewRequest("GET", LoginURL, nil)
	c.setCommonHeaders(req)
	
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// Step 2: Solve Captcha
	re := regexp.MustCompile(`What\s+is\s+(\d+)\s*\+\s*(\d+)`)
	matches := re.FindStringSubmatch(bodyString)
	
	if len(matches) < 3 {
		// اگر یہاں 403 آتا ہے تو مطلب IP بلاک ہے
		if strings.Contains(bodyString, "Forbidden") {
			return errors.New("SERVER BLOCKED IP (403 Forbidden)")
		}
		return errors.New("captcha regex failed")
	}

	num1, _ := strconv.Atoi(matches[1])
	num2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(num1 + num2)
	fmt.Printf("[Masdar] Captcha Solved: %s + %s = %s\n", matches[1], matches[2], captchaAns)

	// Step 3: Post Login
	data := url.Values{}
	data.Set("username", "Kami526") 
	data.Set("password", "Kami526") 
	data.Set("capt", captchaAns)

	loginReq, _ := http.NewRequest("POST", SigninURL, bytes.NewBufferString(data.Encode()))
	c.setCommonHeaders(loginReq)
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("Referer", LoginURL)
	loginReq.Header.Set("Origin", BaseURL)

	resp, err = c.HTTPClient.Do(loginReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Step 4: Extract Csstr Token from Reports Page
	// لاگ ان کے بعد یہ ٹوکن نکالنا ضروری ہے ورنہ API نہیں چلے گی
	reportReq, _ := http.NewRequest("GET", ReportsPage, nil)
	c.setCommonHeaders(reportReq)
	reportReq.Header.Set("Referer", BaseURL+"/ints/agent/MySMSNumbers")

	resp, err = c.HTTPClient.Do(reportReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reportBody, _ := io.ReadAll(resp.Body)
	reportString := string(reportBody)
	
	csstrRe := regexp.MustCompile(`csstr=([a-zA-Z0-9]+)`)
	csstrMatch := csstrRe.FindStringSubmatch(reportString)
	
	if len(csstrMatch) > 1 {
		c.Csstr = csstrMatch[1]
		fmt.Println("[Masdar] LOGIN SUCCESS. Token Saved:", c.Csstr)
	} else {
		// Fallback for different HTML structure
		fallbackRe := regexp.MustCompile(`["']csstr["']\s*[:=]\s*["']?([^"']+)["']?`)
		match2 := fallbackRe.FindStringSubmatch(reportString)
		if len(match2) > 1 {
			c.Csstr = match2[1]
			fmt.Println("[Masdar] LOGIN SUCCESS (Fallback). Token Saved:", c.Csstr)
		} else {
			return errors.New("failed to extract csstr token after login")
		}
	}

	return nil
}

// ---------------------- API CALLS ----------------------

func (c *Client) GetSMSLogs() ([]byte, error) {
	// 1. Ensure we are logged in (Only runs if c.Csstr is empty)
	if err := c.ensureSession(); err != nil {
		return nil, err
	}

	// 2. Prepare API Request
	now := time.Now()
	todayDate := now.Format("2006-01-02")
	fdate1 := todayDate + " 00:00:00"
	fdate2 := todayDate + " 23:59:59"

	params := url.Values{}
	params.Set("fdate1", fdate1)
	params.Set("fdate2", fdate2)
	params.Set("frange", "")
	params.Set("fclient", "")
	params.Set("fg", "0")
	params.Set("csstr", c.Csstr) // Saved Token Used Here
	params.Set("sEcho", "1")
	params.Set("iDisplayStart", "0")
	params.Set("iDisplayLength", "100") 
	params.Set("mDataProp_0", "0") 

	finalURL := SMSApiURL + "?" + params.Encode()

	req, _ := http.NewRequest("GET", finalURL, nil)
	c.setCommonHeaders(req)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", ReportsPage)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")

	// 3. Execute Request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// 4. Validate Response (Check if Session Expired)
	if bytes.Contains(body, []byte("<!DOCTYPE html>")) || bytes.Contains(body, []byte("<html")) {
		fmt.Println("[Masdar] Session Expired/Invalid. Clearing token...")
		
		// سیشن ایکسپائر ہو گیا ہے، ٹوکن خالی کرو تاکہ اگلی بار نیا لاگ ان ہو
		c.Mutex.Lock()
		c.Csstr = "" 
		c.HTTPClient.Jar, _ = cookiejar.New(nil) // Clear cookies
		c.Mutex.Unlock()

		// ایک بار دوبارہ ٹرائی کرو (Recursion)
		fmt.Println("[Masdar] Retrying with new login...")
		time.Sleep(2 * time.Second) // تھوڑا انتظار کرو
		return c.GetSMSLogs()
	}

	return cleanMasdarSMS(body)
}

func (c *Client) GetNumberStats() ([]byte, error) {
	// نمبرز والی API کے لیے بھی سیشن چیک کرو
	if err := c.ensureSession(); err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("frange", "")
	params.Set("fclient", "")
	params.Set("csstr", c.Csstr) // Saved Token
	params.Set("sEcho", "2")
	params.Set("iDisplayStart", "0")
	params.Set("iDisplayLength", "-1") 
	params.Set("sSortDir_0", "asc")

	finalURL := NumberApiURL + "?" + params.Encode()

	req, _ := http.NewRequest("GET", finalURL, nil)
	c.setCommonHeaders(req)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Referer", BaseURL+"/ints/agent/MySMSNumbers")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// Check expiry
	if bytes.Contains(body, []byte("<!DOCTYPE html>")) || bytes.Contains(body, []byte("<html")) {
		fmt.Println("[Masdar] Number API Session Invalid. Clearing...")
		c.Mutex.Lock()
		c.Csstr = ""
		c.HTTPClient.Jar, _ = cookiejar.New(nil)
		c.Mutex.Unlock()
		
		time.Sleep(2 * time.Second)
		return c.GetNumberStats()
	}

	return cleanMasdarNumbers(body)
}

// Helper to clean up repeated headers
func (c *Client) setCommonHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,ur-PK;q=0.8,ur;q=0.7")
	req.Header.Set("Connection", "keep-alive")
}

// ------------------ CLEANING FUNCTIONS (No Changes) ------------------

func cleanMasdarSMS(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}
	var cleanedRows [][]interface{}
	for _, row := range apiResp.AAData {
		if len(row) > 8 {
			msg, _ := row[5].(string)
			msg = html.UnescapeString(msg)
			msg = strings.ReplaceAll(msg, "null", "")
			newRow := []interface{}{
				row[0], row[1], row[2], row[3], msg, row[6], row[7], row[8],
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}
	apiResp.AAData = cleanedRows
	return json.Marshal(apiResp)
}

func cleanMasdarNumbers(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}
	var cleanedRows [][]interface{}
	rePrice := regexp.MustCompile(`[\d\.]+`)
	for _, row := range apiResp.AAData {
		if len(row) > 7 {
			rangeName := row[1]
			prefix := row[2]
			number := row[3]
			priceHTML, _ := row[4].(string)
			billingType := "Weekly"
			if strings.Contains(strings.ToLower(priceHTML), "monthly") {
				billingType = "Monthly"
			}
			currency := "$"
			if strings.Contains(priceHTML, "€") { currency = "€" } else if strings.Contains(priceHTML, "£") { currency = "£" }
			priceVal := "0"
			matches := rePrice.FindAllString(priceHTML, -1)
			if len(matches) > 0 { priceVal = matches[len(matches)-1] }
			priceStr := currency + " " + priceVal
			stats := row[7]
			newRow := []interface{}{
				rangeName, prefix, number, billingType, priceStr, stats,
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}
	apiResp.AAData = cleanedRows
	apiResp.ITotalRecords = len(cleanedRows)
	apiResp.ITotalDisplayRecords = len(cleanedRows)
	return json.Marshal(apiResp)
}