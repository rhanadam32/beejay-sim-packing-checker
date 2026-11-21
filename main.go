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
	"time"

	"github.com/ledongthuc/pdf"
)

type Master struct {
	ID        int      `json:"id"`
	ChatName  string   `json:"chat_name"`
	AppsName  string   `json:"apps_name"`
	AliasChat []string `json:"alias_chat"`
	Version   int      `json:"version"`
}

type Row struct {
	ID        int     `json:"id"`
	ChatName  string  `json:"chat_name"`
	AppsName  string  `json:"apps_name"`
	ChatQty   float64 `json:"chat_qty"`
	ChatFound bool    `json:"chat_found"`
	AppsQty   float64 `json:"apps_qty"`
	AppsFound bool    `json:"apps_found"`
	Status    string  `json:"status"`
	Delta     float64 `json:"delta"`
}

type Summary struct {
	OK           int     `json:"ok"`
	Mismatch     int     `json:"mismatch"`
	OnlyChat     int     `json:"only_chat"`
	OnlyApps     int     `json:"only_apps"`
	TotalChatQty float64 `json:"total_chat_qty"`
	TotalAppsQty float64 `json:"total_apps_qty"`
	TotalDelta   float64 `json:"total_delta"`
}

type CompareResponse struct {
	Rows    []Row     `json:"rows"`
	Summary Summary   `json:"summary"`
}

type FieldIssue struct {
	Field     string `json:"field"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	MasterID  int    `json:"master_id,omitempty"`
}

type UploadFileResult struct {
	Name   string `json:"name"`
	Count  int    `json:"count"`
	Error  string `json:"error,omitempty"`
	Dup    bool   `json:"duplicate,omitempty"`
}

var (
	mu           sync.Mutex
	masters      []Master
	chatMap      = map[string]float64{}
	chatOrig     = map[string]string{}
	appsMap      = map[string]float64{}
	appsAllMap   = map[string]float64{}
	appsAllOrig  = map[string]string{}
	appsFile     = ""
	uploadedPDFs = map[string]bool{}
	undoStack    [][]Master
	redoStack    [][]Master
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

func findChatQty(name string) (float64, bool) {
	key := normalize(name)
	if v, ok := chatMap[key]; ok {
		return v, true
	}
	return 0, false
}

func findAppsQty(name string) (float64, bool) {
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
		m := Master{
			ID:        id,
			ChatName:  strings.TrimSpace(row[1]),
			AppsName:  strings.TrimSpace(row[2]),
			AliasChat: []string{},
			Version:   1,
		}
		if len(row) >= 4 && strings.TrimSpace(row[3]) != "" {
			for _, a := range strings.Split(row[3], "|") {
				a = strings.TrimSpace(a)
				if a != "" {
					m.AliasChat = append(m.AliasChat, a)
				}
			}
		}
		masters = append(masters, m)
	}
	return nil
}

func saveMaster() error {
	var buf strings.Builder
	w := csv.NewWriter(&nopWriter{&buf})
	_ = w
	// build via bytes
	var sb strings.Builder
	cw := csv.NewWriter(stringWriter{&sb})
	cw.Comma = ';'
	cw.Write([]string{"ide", "master_product_chat", "master_product_apps", "alias_chat"})
	for _, m := range masters {
		cw.Write([]string{strconv.Itoa(m.ID), m.ChatName, m.AppsName, strings.Join(m.AliasChat, "|")})
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return err
	}
	return writeFileAtomic("database/data master.csv", []byte(sb.String()))
}

type stringWriter struct{ *strings.Builder }

func (s stringWriter) Write(p []byte) (int, error) { return s.Builder.Write(p) }

type nopWriter struct{ *strings.Builder }

func (n nopWriter) Write(p []byte) (int, error) { return n.Builder.Write(p) }

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("gagal menulis berkas sementara: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		backup := fmt.Sprintf("%s.%s.bak", path, time.Now().Format("20060102-150405"))
		if b, err := os.ReadFile(path); err == nil {
			_ = os.WriteFile(backup, b, 0o644)
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("gagal mengganti berkas master (tutup berkas bila sedang dibuka di Excel): %w", err)
	}
	return nil
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

func parseChat(text string, orig map[string]string) map[string]float64 {
	m := map[string]float64{}
	inLuke := false
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			inLuke = false
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
		if !strings.Contains(strings.ToLower(valPart), "mc") {
			continue
		}
		re := regexp.MustCompile(`[\d]+(?:[.,][\d]+)?`)
		numStr := re.FindString(valPart)
		if numStr == "" {
			continue
		}
		numStr = strings.ReplaceAll(numStr, ",", ".")
		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			continue
		}
		if inLuke && !strings.HasPrefix(strings.ToLower(name), "luke") {
			name = "Luke " + name
		}
		m[normalize(name)] += num
		orig[normalize(name)] = name
	}
	return m
}

func isSizeToken(s string) bool {
	return regexp.MustCompile(`^[0-9][0-9/x+]*$`).MatchString(s)
}

func parseAppsPDF(text string) map[string]float64 {
	result := map[string]float64{}
	normText := strings.ToLower(normalize(text))
	for _, m := range masters {
		if strings.TrimSpace(m.AppsName) == "" {
			continue
		}
		name := strings.ToLower(normalize(m.AppsName))
		idx := strings.Index(normText, name)
		if idx < 0 {
			continue
		}
		after := normText[idx+len(name):]
		re := regexp.MustCompile(`^\s*\S+\s+(\d+)`)
		mm := re.FindStringSubmatch(after)
		if mm != nil {
			qty, _ := strconv.ParseFloat(mm[1], 64)
			result[normalize(m.AppsName)] += qty
		}
	}
	return result
}

func parseAppsPDFAll(text string, orig map[string]string) map[string]float64 {
	result := map[string]float64{}
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
		i++
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
		q, _ := strconv.ParseFloat(qtyStr, 64)
		result[name] += q
		if _, ok := orig[name]; !ok {
			orig[name] = strings.Join(nameParts, " ")
		}
	}
	return result
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

func unmatchedChat() []string {
	var out []string
	for k, orig := range chatOrig {
		matched := false
		for _, m := range masters {
			if normalize(k) == normalize(m.ChatName) {
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
			if normalize(k) == normalize(m.AppsName) {
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

func levenshteinCandidates(target string, list []string) []string {
	type cand struct {
		name string
		dist int
	}
	var cands []cand
	for _, n := range list {
		d := levenshtein(normalize(target), normalize(n))
		if d <= 6 {
			cands = append(cands, cand{n, d})
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].dist < cands[j].dist })
	var out []string
	for i, c := range cands {
		if i >= 3 {
			break
		}
		out = append(out, c.name)
	}
	return out
}

func chatQtyFor(m Master) (float64, bool) {
	qty := 0.0
	found := false
	names := append([]string{m.ChatName}, m.AliasChat...)
	for _, n := range names {
		if strings.TrimSpace(n) == "" {
			continue
		}
		if v, ok := chatMap[normalize(n)]; ok {
			qty += v
			found = true
		}
	}
	return qty, found
}

func buildCompare() CompareResponse {
	rows := []Row{}
	for _, m := range masters {
		cq, cf := chatQtyFor(m)
		aq, af := findAppsQty(m.AppsName)
		status := ""
		if cf && af {
			if cq == aq {
				status = "OK"
			} else {
				status = "MISMATCH"
			}
		} else if cf && !af {
			status = "HANYA DI CHAT"
		} else if !cf && af {
			status = "HANYA DI APPS"
		} else {
			continue
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
			Delta:     cq - aq,
		})
	}
	order := map[string]int{"MISMATCH": 0, "HANYA DI CHAT": 1, "HANYA DI APPS": 2, "OK": 3}
	sort.Slice(rows, func(i, j int) bool {
		if order[rows[i].Status] != order[rows[j].Status] {
			return order[rows[i].Status] < order[rows[j].Status]
		}
		return rows[i].ID < rows[j].ID
	})
	summary := Summary{}
	for _, r := range rows {
		switch r.Status {
		case "OK":
			summary.OK++
		case "MISMATCH":
			summary.Mismatch++
		case "HANYA DI CHAT":
			summary.OnlyChat++
		case "HANYA DI APPS":
			summary.OnlyApps++
		}
		summary.TotalChatQty += r.ChatQty
		summary.TotalAppsQty += r.AppsQty
		summary.TotalDelta += r.Delta
	}
	return CompareResponse{Rows: rows, Summary: summary}
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
	chatMap = parseChat(text, chatOrig)
	mu.Unlock()
	writeJSON(w, map[string]interface{}{"count": len(chatMap)})
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
	var results []UploadFileResult
	mu.Lock()
	for _, fh := range files {
		res := UploadFileResult{Name: fh.Filename}
		dup := uploadedPDFs[fh.Filename]
		if dup {
			res.Dup = true
		}
		f, err := fh.Open()
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
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
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		parsed := parseAppsPDF(text)
		allParsed := parseAppsPDFAll(text, appsAllOrig)
		count := 0
		for k, v := range parsed {
			appsMap[k] += v
			count++
		}
		for k, v := range allParsed {
			appsAllMap[k] += v
			if _, ok := appsAllOrig[k]; !ok {
				appsAllOrig[k] = k
			}
		}
		res.Count = count
		if count == 0 {
			res.Error = "0 item terbaca (format PDF berubah atau scan)"
		}
		uploadedPDFs[fh.Filename] = true
		names = append(names, fh.Filename)
		results = append(results, res)
	}
	if appsFile == "" {
		appsFile = strings.Join(names, ", ")
	} else {
		appsFile = appsFile + ", " + strings.Join(names, ", ")
	}
	total := len(appsMap)
	mu.Unlock()
	writeJSON(w, map[string]interface{}{
		"count":   total,
		"file":    appsFile,
		"files":   results,
	})
}

func handleResetChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	mu.Lock()
	chatMap = map[string]float64{}
	chatOrig = map[string]string{}
	mu.Unlock()
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleResetPDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	mu.Lock()
	appsMap = map[string]float64{}
	appsAllMap = map[string]float64{}
	appsAllOrig = map[string]string{}
	appsFile = ""
	uploadedPDFs = map[string]bool{}
	mu.Unlock()
	writeJSON(w, map[string]string{"status": "ok"})
}

func cloneMasters() []Master {
	out := make([]Master, len(masters))
	copy(out, masters)
	for i := range out {
		out[i].AliasChat = append([]string{}, masters[i].AliasChat...)
	}
	return out
}

func pushSnapshot() {
	undoStack = append(undoStack, cloneMasters())
	if len(undoStack) > 20 {
		undoStack = undoStack[len(undoStack)-20:]
	}
	redoStack = nil
}

func validateMaster(m Master, others []Master) []FieldIssue {
	var issues []FieldIssue
	m.ChatName = strings.Join(strings.Fields(m.ChatName), " ")
	m.AppsName = strings.Join(strings.Fields(m.AppsName), " ")
	if m.ChatName == "" && m.AppsName == "" {
		issues = append(issues, FieldIssue{"chat_name", "error", "Isi minimal satu nama.", 0})
		return issues
	}
	if len(m.ChatName) > 200 {
		issues = append(issues, FieldIssue{"chat_name", "error", "Nama terlalu panjang.", 0})
	}
	if len(m.AppsName) > 200 {
		issues = append(issues, FieldIssue{"apps_name", "error", "Nama terlalu panjang.", 0})
	}
	for _, o := range others {
		if o.ID == m.ID {
			continue
		}
		if m.ChatName != "" && normalize(m.ChatName) == normalize(o.ChatName) {
			issues = append(issues, FieldIssue{"chat_name", "error", fmt.Sprintf("Nama sudah dipakai master #%d.", o.ID), o.ID})
		} else if m.ChatName != "" && similarity(m.ChatName, o.ChatName) >= 0.9 {
			issues = append(issues, FieldIssue{"chat_name", "warning", fmt.Sprintf("Mirip dengan master #%d.", o.ID), o.ID})
		}
		if m.AppsName != "" && normalize(m.AppsName) == normalize(o.AppsName) {
			issues = append(issues, FieldIssue{"apps_name", "error", fmt.Sprintf("Nama sudah dipakai master #%d.", o.ID), o.ID})
		} else if m.AppsName != "" && similarity(m.AppsName, o.AppsName) >= 0.9 {
			issues = append(issues, FieldIssue{"apps_name", "warning", fmt.Sprintf("Mirip dengan master #%d.", o.ID), o.ID})
		}
	}
	return issues
}

func similarity(a, b string) float64 {
	a, b = normalize(a), normalize(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	ra, rb := []rune(a), []rune(b)
	lev := 1 - float64(levenshteinRunes(ra, rb))/float64(max(len(ra), len(rb)))
	return 0.6*lev + 0.4*tokenJaccard(a, b)
}

func tokenJaccard(a, b string) float64 {
	sa, sb := map[string]bool{}, map[string]bool{}
	for _, t := range strings.Fields(a) {
		sa[t] = true
	}
	for _, t := range strings.Fields(b) {
		sb[t] = true
	}
	inter := 0
	for t := range sa {
		if sb[t] {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func levenshteinRunes(a, b []rune) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
		issues := validateMaster(m, masters)
		hasErr := false
		for _, is := range issues {
			if is.Severity == "error" {
				hasErr = true
			}
		}
		if hasErr {
			writeJSON(w, map[string]interface{}{"issues": issues})
			return
		}
		pushSnapshot()
		m.ID = nextID()
		m.Version = 1
		masters = append(masters, m)
		if err := saveMaster(); err != nil {
			http.Error(w, "gagal simpan: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]interface{}{"master": m, "issues": issues})
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
		issues := validateMaster(m, masters)
		hasErr := false
		for _, is := range issues {
			if is.Severity == "error" {
				hasErr = true
			}
		}
		if hasErr {
			writeJSON(w, map[string]interface{}{"issues": issues})
			return
		}
		pushSnapshot()
		m.ID = id
		m.Version = masters[idx].Version + 1
		masters[idx] = m
		if err := saveMaster(); err != nil {
			http.Error(w, "gagal simpan: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]interface{}{"master": m, "issues": issues})
	case http.MethodDelete:
		pushSnapshot()
		masters = append(masters[:idx], masters[idx+1:]...)
		if err := saveMaster(); err != nil {
			http.Error(w, "gagal simpan: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method", 405)
	}
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

func handleUndo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if len(undoStack) == 0 {
		writeJSON(w, map[string]string{"status": "nothing"})
		return
	}
	redoStack = append(redoStack, cloneMasters())
	masters = undoStack[len(undoStack)-1]
	undoStack = undoStack[:len(undoStack)-1]
	saveMaster()
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleRedo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if len(redoStack) == 0 {
		writeJSON(w, map[string]string{"status": "nothing"})
		return
	}
	undoStack = append(undoStack, cloneMasters())
	masters = redoStack[len(redoStack)-1]
	redoStack = redoStack[:len(redoStack)-1]
	saveMaster()
	writeJSON(w, map[string]string{"status": "ok"})
}

func handlePairingQueue(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	type suggestion struct {
		Type     string  `json:"type"`
		Name     string  `json:"name,omitempty"`
		MasterID int     `json:"master_id,omitempty"`
		Score    float64 `json:"score"`
	}
	type item struct {
		Name         string       `json:"name"`
		Qty          float64      `json:"qty"`
		Suggestions  []suggestion `json:"suggestions"`
	}
	chatUnmatched := []item{}
	for _, name := range unmatchedChat() {
		qty := chatMap[normalize(name)]
		var appsNames []string
		for _, a := range unmatchedApps() {
			appsNames = append(appsNames, a)
		}
		var masterNames []string
		for _, m := range masters {
			masterNames = append(masterNames, m.ChatName)
		}
		var sugs []suggestion
		for _, a := range appsNames {
			sc := similarity(name, a)
			if sc >= 0.6 {
				sugs = append(sugs, suggestion{Type: "pair", Name: a, Score: sc})
			}
		}
		for _, mn := range masterNames {
			sc := similarity(name, mn)
			if sc >= 0.6 {
				for _, m := range masters {
					if m.ChatName == mn {
						sugs = append(sugs, suggestion{Type: "alias", MasterID: m.ID, Score: sc})
					}
				}
			}
		}
		if len(sugs) == 0 {
			sugs = append(sugs, suggestion{Type: "new", Score: 0})
		}
		chatUnmatched = append(chatUnmatched, item{Name: name, Qty: qty, Suggestions: sugs})
	}
	writeJSON(w, map[string]interface{}{"chat_unmatched": chatUnmatched, "apps_unmatched": unmatchedApps()})
}

func handlePairingAccept(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	var body struct {
		Type     string  `json:"type"`
		ChatName string  `json:"chat_name"`
		AppsName string  `json:"apps_name"`
		MasterID int     `json:"master_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	pushSnapshot()
	if body.Type == "alias" {
		for i := range masters {
			if masters[i].ID == body.MasterID {
				masters[i].AliasChat = append(masters[i].AliasChat, body.ChatName)
				saveMaster()
				writeJSON(w, masters[i])
				return
			}
		}
	}
	m := Master{ID: nextID(), ChatName: body.ChatName, AppsName: body.AppsName, AliasChat: []string{}, Version: 1}
	masters = append(masters, m)
	saveMaster()
	writeJSON(w, m)
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
	pushSnapshot()
	m := Master{ID: nextID(), ChatName: body.ChatName, AppsName: body.AppsName, AliasChat: []string{}, Version: 1}
	masters = append(masters, m)
	if err := saveMaster(); err != nil {
		http.Error(w, "gagal simpan: "+err.Error(), 500)
		return
	}
	writeJSON(w, m)
}

func handleCandidates(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	side := r.URL.Query().Get("side")
	mu.Lock()
	defer mu.Unlock()
	var list []string
	if side == "chat" {
		for _, m := range masters {
			list = append(list, m.ChatName)
		}
	} else {
		for _, m := range masters {
			list = append(list, m.AppsName)
		}
	}
	cands := levenshteinCandidates(name, list)
	writeJSON(w, cands)
}

func handleNames(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	q := strings.ToLower(r.URL.Query().Get("q"))
	mu.Lock()
	defer mu.Unlock()
	set := map[string]bool{}
	if source == "chat" {
		for k, orig := range chatOrig {
			if q == "" || strings.Contains(strings.ToLower(orig), q) || strings.Contains(k, q) {
				set[orig] = true
			}
		}
	} else {
		for k, orig := range appsAllOrig {
			if q == "" || strings.Contains(strings.ToLower(orig), q) || strings.Contains(k, q) {
				set[orig] = true
			}
		}
	}
	var out []string
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	if len(out) > 50 {
		out = out[:50]
	}
	writeJSON(w, out)
}

func main() {
	if err := loadMaster("database/data master.csv"); err != nil {
		fmt.Println("PERINGATAN: data master.csv tidak ditemukan:", err)
	}
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
	mux.HandleFunc("/api/candidates", handleCandidates)
	mux.HandleFunc("/api/masters/undo", handleUndo)
	mux.HandleFunc("/api/masters/redo", handleRedo)
	mux.HandleFunc("/api/pairing/queue", handlePairingQueue)
	mux.HandleFunc("/api/pairing/accept", handlePairingAccept)
	mux.HandleFunc("/api/names", handleNames)

	fmt.Println("Server running at http://localhost:8080")
	http.ListenAndServe(":8080", mux)
}