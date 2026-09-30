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

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var locationPattern = regexp.MustCompile(`^[1-9][0-9]*\.[1-9][0-9]*\.[1-9][0-9]*$`)

type input map[string]any

func (in input) str(k string) string { s, _ := in[k].(string); return s }

// A '?' suffix marks optional fields. Identifiers remain literal strings.
func decodeInput(w http.ResponseWriter, r *http.Request, fields string, keyRequired bool) (input, error) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		return nil, invalid("Query-параметры команды запрещены")
	}
	if keyRequired {
		keys := r.Header.Values("Idempotency-Key")
		if len(keys) != 1 || !uuidPattern.MatchString(keys[0]) {
			return nil, invalid("Нужен UUID Idempotency-Key")
		}
		r.Header.Set("Idempotency-Key", strings.ToLower(keys[0]))
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return nil, fail(415, "UNSUPPORTED_MEDIA_TYPE", "Ожидается application/json")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		return nil, fail(413, "PAYLOAD_TOO_LARGE", "Максимальный размер тела — 64 KiB")
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
		allowed[strings.TrimSuffix(f, "?")] = !strings.HasSuffix(f, "?")
	}
	in := input{}
	for dec.More() {
		tok, err = dec.Token()
		if err != nil {
			return nil, invalid("Некорректный JSON")
		}
		k, ok := tok.(string)
		if _, exists := allowed[k]; !ok || !exists {
			return nil, invalid("Неизвестное поле")
		}
		if _, ok := in[k]; ok {
			return nil, invalid("Повторяющееся поле JSON")
		}
		var v any
		if err := dec.Decode(&v); err != nil || v == nil {
			return nil, invalid("Неверное значение поля")
		}
		switch k {
		case "is_active":
			if _, ok := v.(bool); !ok {
				return nil, invalid("is_active должен быть boolean")
			}
		case "expected_version":
			n, ok := v.(json.Number)
			if !ok {
				return nil, invalid("expected_version должен быть integer")
			}
			i, err := n.Int64()
			if err != nil {
				return nil, invalid("expected_version должен быть int64")
			}
			if i < 1 {
				return nil, fail(422, "VALIDATION_FAILED", "Версия должна быть положительной")
			}
			v = i
		default:
			s, ok := v.(string)
			if !ok {
				return nil, invalid("Ожидается строка")
			}
			if strings.TrimSpace(s) == "" || strings.ContainsRune(s, 0) {
				return nil, fail(422, "VALIDATION_FAILED", "Пустая строка или запрещённый символ")
			}
			if strings.HasSuffix(k, "_location") && !locationPattern.MatchString(s) {
				return nil, fail(422, "VALIDATION_FAILED", "Адрес: три положительных числа через точку, без ведущих нулей")
			}
			if k == "replaces_credential_id" {
				if !uuidPattern.MatchString(s) {
					return nil, invalid("Неверный UUID")
				}
				s = strings.ToLower(s)
			}
			v = s
		}
		in[k] = v
	}
	if _, err := dec.Token(); err != nil {
		return nil, invalid("Некорректный JSON")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, invalid("Лишние данные после JSON")
	}
	for k, required := range allowed {
		if _, ok := in[k]; required && !ok {
			return nil, invalid("Отсутствует обязательное поле")
		}
	}
	return in, nil
}
