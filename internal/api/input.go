package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

var codePattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type input map[string]any

func (in input) str(key string) string { s, _ := in[key].(string); return s }

// decodeInput rejects duplicate, unknown, null and missing fields before hashing.
// The map is marshaled in lexicographic key order for canonical command hashes.
func decodeInput(w http.ResponseWriter, r *http.Request, fields string) (input, error) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		return nil, invalid("Query-параметры команды запрещены")
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !codePattern.MatchString(keys[0]) {
		return nil, invalid("Нужен корректный Idempotency-Key")
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return nil, fail(415, "UNSUPPORTED_MEDIA_TYPE", "Ожидается application/json")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		return nil, fail(413, "REQUEST_TOO_LARGE", "Максимальный размер тела — 64 KiB")
	}
	if !utf8.Valid(body) {
		return nil, invalid("Ожидается UTF-8 JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, invalid("Ожидается JSON-объект")
	}
	allowed := map[string]bool{}
	for _, f := range strings.Fields(fields) {
		allowed[f] = true
	}
	in := input{}
	for dec.More() {
		tok, err = dec.Token()
		if err != nil {
			return nil, invalid("Некорректный JSON")
		}
		key, ok := tok.(string)
		if !ok || !allowed[key] {
			return nil, invalid("Неизвестное поле")
		}
		if _, ok := in[key]; ok {
			return nil, invalid("Повторяющееся поле JSON")
		}
		var value any
		if err := dec.Decode(&value); err != nil || value == nil {
			return nil, invalid("Неверное значение поля")
		}
		if key == "number" {
			n, ok := value.(json.Number)
			if !ok {
				return nil, invalid("number должен быть целым числом")
			}
			i, err := n.Int64()
			if err != nil || i < 1 || i > 2147483647 {
				return nil, invalid("number вне диапазона int32")
			}
			in[key] = int32(i)
			continue
		}
		s, ok := value.(string)
		if !ok {
			return nil, invalid("Ожидается строка")
		}
		switch key {
		case "name", "reason":
			s = strings.TrimSpace(s)
			max := 200
			if key == "reason" {
				max = 500
			}
			if utf8.RuneCountInString(s) < 1 || utf8.RuneCountInString(s) > max || strings.ContainsRune(s, 0) {
				return nil, invalid("Неверная длина или содержимое строки")
			}
		case "cell_id", "target_cell_id":
			if !uuidPattern.MatchString(s) {
				return nil, invalid("Неверный UUID")
			}
			s = strings.ToLower(s)
		default:
			if !codePattern.MatchString(s) {
				return nil, invalid("Неверный формат кода")
			}
		}
		in[key] = s
	}
	if _, err := dec.Token(); err != nil {
		return nil, invalid("Некорректный JSON")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, invalid("Лишние данные после JSON")
	}
	if len(in) != len(allowed) {
		return nil, invalid("Отсутствует обязательное поле")
	}
	return in, nil
}
