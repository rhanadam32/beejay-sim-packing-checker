package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ledongthuc/pdf"
)

type Master struct {
	ID       int    `json:"id"`
	ChatName string `json:"chat_name"`
	AppsName string `json:"apps_name"`
}

type Row struct {
	ID        int    `json:"id"`
	ChatName  string `json:"chat_name"`
	AppsName  string `json:"apps_name"`
	ChatQty   int    `json:"chat_qty"`
	ChatFound bool   `json:"chat_found"`
	AppsQty   int    `json:"apps_qty"`
	AppsFound bool   `json:"apps_found"`
	Status    string `json:"status"`
}

var (
	mu           sync.Mutex
	masters      []Master
	chatMap      = map[string]int{}
	chatOrig     = map[string]string{}
	appsMap      = map[string]int{}
	appsAllMap   = map[string]int{}
	appsAllOrig  = map[string]string{}
	appsFile     = ""
)

func normalize(s string) string {
	s = strings.ReplaceAll(s, "ﬁ", "fi")
	s = strings.ReplaceAll(s, "ﬂ", "fl")
	s = strings.ToLower(strings.TrimSpace(s))
	re := regexp.MustCompile(`\s+`)
	s = re.ReplaceAllString(s, " ")
	return s
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur := make([]int, lb+1)
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func findChatQty(name string) (int, bool) {
	key := normalize(name)
	if v, ok := chatMap[key]; ok {
		return v, true
	}
	return 0, false
}

func findAppsQty(name string) (int, bool) {
	key := normalize(name)
	if v, ok := appsMap[key]; ok {
		return v, true
	}
	return 0, false
}

func loadMaster(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReader(f))
	r.Comma = ';'
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return err
	}
	masters = nil
	for i, row := range rows {
		if i == 0 {
			continue
		}
		if len(row) < 3 {
			continue
		}
		id, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(row[0], "\ufeff")))
		masters = append(masters, Master{
			ID:       id,
			ChatName: strings.TrimSpace(row[1]),
			AppsName: strings.TrimSpace(row[2]),
		})
	}
	return nil
}

func saveMaster() {
	f, err := os.Create("database/data master.csv")
	if err != nil {
		return
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Comma = ';'
	w.Write([]string{"ide", "master_product_chat", "master_product_apps"})
	for _, m := range masters {
		w.Write([]string{strconv.Itoa(m.ID), m.ChatName, m.AppsName})
	}
	w.Flush()
}

func nextID() int {
	max := 0
	for _, m := range masters {
		if m.ID > max {
			max = m.ID
		}
	}
	return max + 1
}

func parseChat(text string, orig map[string]string) map[string]int {
	m := map[string]int{}
	inLuke := false
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.EqualFold(line, "Luke") {
			inLuke = true
			continue
		}
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		name := strings.TrimSpace(parts[0])
		if strings.EqualFold(name, "Total") {
			continue
		}
		valPart := strings.TrimSpace(parts[1])
		re := regexp.MustCompile(`\d+`)
		numStr := re.FindString(valPart)
		if numStr == "" {
			continue
		}
		num, _ := strconv.Atoi(numStr)
		if inLuke && !strings.HasPrefix(strings.ToLower(name), "luke") {
			name = "Luke " + name
		}
		m[normalize(name)] = num
		orig[normalize(name)] = name
	}
	return m
}

func parseAppsPDF(text string) map[string]int {
	result := map[string]int{}
	normText := strings.ToLower(normalize(text))
	for _, m := range masters {
		name := strings.ToLower(normalize(m.AppsName))
		idx := strings.Index(normText, name)
		if idx < 0 {
			continue
		}
		after := normText[idx+len(name):]
		re := regexp.MustCompile(`^\s*\S+\s+(\d+)`)
		mm := re.FindStringSubmatch(after)
		if mm != nil {
			qty, _ := strconv.Atoi(mm[1])
			result[normalize(m.AppsName)] = qty
		}
	}
	return result
}

func isSizeToken(s string) bool {
	return regexp.MustCompile(`^[0-9][0-9/x+]*$`).MatchString(s)
}

func parseAppsPDFAll(text string, orig map[string]string) map[string]int {
	result := map[string]int{}
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		cleaned = append(cleaned, l)
	}
	i := 0
	for i < len(cleaned) {
		var nameParts []string
		for i < len(cleaned) && !isSizeToken(cleaned[i]) {
			if len(nameParts) == 0 && !strings.Contains(cleaned[i], ",") {
				i++
				continue
			}
			nameParts = append(nameParts, cleaned[i])
			i++
		}
		if i >= len(cleaned) {
			break
		}
		i++ // skip size
		qtyStr := ""
		for i < len(cleaned) && qtyStr == "" {
			if regexp.MustCompile(`^\d+$`).MatchString(cleaned[i]) {
				qtyStr = cleaned[i]
			}
			i++
		}
		name := normalize(strings.Join(nameParts, " "))
		if name == "" || !strings.Contains(name, ",") {
			continue
		}
		q, _ := strconv.Atoi(qtyStr)
		result[name] += q
		if _, ok := orig[name]; !ok {
			orig[name] = strings.Join(nameParts, " ")
		}
	}
	return result
}

func unmatchedChat() []string {
	var out []string
	for k, orig := range chatOrig {
		matched := false
		for _, m := range masters {
			if levenshtein(k, normalize(m.ChatName)) == 0 {
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, orig)
		}
	}
	return out
}

func unmatchedApps() []string {
	var out []string
	for k, orig := range appsAllOrig {
		matched := false
		for _, m := range masters {
			if levenshtein(k, normalize(m.AppsName)) == 0 {
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, orig)
		}
	}
	return out
}

func handleUnmatched(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	chat := unmatchedChat()
	apps := unmatchedApps()
	if chat == nil {
		chat = []string{}
	}
	if apps == nil {
		apps = []string{}
	}
	writeJSON(w, map[string]interface{}{
		"chat": chat,
		"apps": apps,
	})
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	var body struct {
		ChatName string `json:"chat_name"`
		AppsName string `json:"apps_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	m := Master{ID: nextID(), ChatName: body.ChatName, AppsName: body.AppsName}
	masters = append(masters, m)
	saveMaster()
	writeJSON(w, m)
}

func extractPDFText(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	reader, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	b, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func buildCompare() []Row {
	rows := []Row{}
	for _, m := range masters {
		cq, cf := findChatQty(m.ChatName)
		aq, af := findAppsQty(m.AppsName)
		status := "OK"
		if !cf || !af {
			status = "MISSING"
		} else if cq != aq {
			status = "MISMATCH"
		}
		rows = append(rows, Row{
			ID:        m.ID,
			ChatName:  m.ChatName,
			AppsName:  m.AppsName,
			ChatQty:   cq,
			ChatFound: cf,
			AppsQty:   aq,
			AppsFound: af,
			Status:    status,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func handleCompare(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	writeJSON(w, buildCompare())
}

func handleUploadChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	var text string
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		text = body.Text
	} else if strings.HasPrefix(ct, "text/plain") {
		b, _ := io.ReadAll(r.Body)
		text = string(b)
	} else {
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		defer file.Close()
		b, _ := io.ReadAll(file)
		text = string(b)
	}
	mu.Lock()
	chatOrig = map[string]string{}
	m := parseChat(text, chatOrig)
	chatMap = m
	mu.Unlock()
	writeJSON(w, map[string]interface{}{"count": len(m)})
}

func handleUploadPDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		http.Error(w, "no file", 400)
		return
	}
	var names []string
	mu.Lock()
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			continue
		}
		tmpDir := os.TempDir()
		tmpPath := filepath.Join(tmpDir, fh.Filename)
		out, _ := os.Create(tmpPath)
		io.Copy(out, f)
		out.Close()
		f.Close()
		text, err := extractPDFText(tmpPath)
		os.Remove(tmpPath)
		if err != nil {
			continue
		}
		parsed := parseAppsPDF(text)
		allParsed := parseAppsPDFAll(text, appsAllOrig)
		for k, v := range parsed {
			appsMap[k] += v
		}
		for k, v := range allParsed {
			appsAllMap[k] += v
			if _, ok := appsAllOrig[k]; !ok {
				appsAllOrig[k] = k
			}
		}
		names = append(names, fh.Filename)
	}
	if appsFile == "" {
		appsFile = strings.Join(names, ", ")
	} else {
		appsFile = appsFile + ", " + strings.Join(names, ", ")
	}
	total := len(appsMap)
	mu.Unlock()
	writeJSON(w, map[string]interface{}{"count": total, "file": appsFile})
}

func handleResetChat(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	chatMap = map[string]int{}
	chatOrig = map[string]string{}
	mu.Unlock()
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleResetPDF(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	appsMap = map[string]int{}
	appsAllMap = map[string]int{}
	appsAllOrig = map[string]string{}
	appsFile = ""
	mu.Unlock()
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleMasters(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, masters)
	case http.MethodPost:
		var m Master
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		m.ID = nextID()
		masters = append(masters, m)
		saveMaster()
		writeJSON(w, m)
	default:
		http.Error(w, "method", 405)
	}
}

func handleMasterItem(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/masters/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	idx := -1
	for i, m := range masters {
		if m.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		http.Error(w, "not found", 404)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var m Master
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		m.ID = id
		masters[idx] = m
		saveMaster()
		writeJSON(w, m)
	case http.MethodDelete:
		masters = append(masters[:idx], masters[idx+1:]...)
		saveMaster()
		writeJSON(w, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method", 405)
	}
}

func main() {
	loadMaster("database/data master.csv")
	if b, err := os.ReadFile("packing from chat/chat.txt"); err == nil {
		mu.Lock()
		chatOrig = map[string]string{}
		chatMap = parseChat(string(b), chatOrig)
		mu.Unlock()
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/index.html")
	})
	mux.HandleFunc("/api/compare", handleCompare)
	mux.HandleFunc("/api/upload-chat", handleUploadChat)
	mux.HandleFunc("/api/upload-pdf", handleUploadPDF)
	mux.HandleFunc("/api/reset-chat", handleResetChat)
	mux.HandleFunc("/api/reset-pdf", handleResetPDF)
	mux.HandleFunc("/api/masters", handleMasters)
	mux.HandleFunc("/api/masters/", handleMasterItem)
	mux.HandleFunc("/api/unmatched", handleUnmatched)
	mux.HandleFunc("/api/masters/register", handleRegister)

	fmt.Println("Server running at http://localhost:8080")
	http.ListenAndServe(":8080", mux)
}
