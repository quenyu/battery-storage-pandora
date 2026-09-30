package handler

import (
	"battery-storage-pandora/internal/model"
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"
)

type input map[string]any

func (in input) str(k string) string { s, _ := in[k].(string); return s }

// A '?' suffix marks optional fields. Identifiers remain literal strings.
func decodeInput(w http.ResponseWriter, r *http.Request, fields string, keyRequired bool) (input, error) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		return nil, model.Invalid("Query-параметры команды запрещены")
	}
	if keyRequired {
		keys := r.Header.Values("Idempotency-Key")
		if len(keys) != 1 || !model.ValidUUID(keys[0]) {
			return nil, model.Invalid("Нужен UUID Idempotency-Key")
		}
		r.Header.Set("Idempotency-Key", strings.ToLower(keys[0]))
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return nil, model.NewError(415, "UNSUPPORTED_MEDIA_TYPE", "Ожидается application/json")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		return nil, model.NewError(413, "PAYLOAD_TOO_LARGE", "Максимальный размер тела — 64 KiB")
	}
	if !utf8.Valid(body) {
		return nil, model.Invalid("Ожидается UTF-8 JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, model.Invalid("Ожидается JSON-объект")
	}
	allowed := map[string]bool{}
	for _, f := range strings.Fields(fields) {
		allowed[strings.TrimSuffix(f, "?")] = !strings.HasSuffix(f, "?")
	}
	in := input{}
	for dec.More() {
		tok, err = dec.Token()
		if err != nil {
			return nil, model.Invalid("Некорректный JSON")
		}
		k, ok := tok.(string)
		if _, exists := allowed[k]; !ok || !exists {
			return nil, model.Invalid("Неизвестное поле")
		}
		if _, ok := in[k]; ok {
			return nil, model.Invalid("Повторяющееся поле JSON")
		}
		var v any
		if err := dec.Decode(&v); err != nil || v == nil {
			return nil, model.Invalid("Неверное значение поля")
		}
		switch k {
		case "is_active":
			if _, ok := v.(bool); !ok {
				return nil, model.Invalid("is_active должен быть boolean")
			}
		case "expected_version":
			n, ok := v.(json.Number)
			if !ok {
				return nil, model.Invalid("expected_version должен быть integer")
			}
			i, err := n.Int64()
			if err != nil {
				return nil, model.Invalid("expected_version должен быть int64")
			}
			if i < 1 {
				return nil, model.NewError(422, "VALIDATION_FAILED", "Версия должна быть положительной")
			}
			v = i
		default:
			s, ok := v.(string)
			if !ok {
				return nil, model.Invalid("Ожидается строка")
			}
			if strings.TrimSpace(s) == "" || strings.ContainsRune(s, 0) {
				return nil, model.NewError(422, "VALIDATION_FAILED", "Пустая строка или запрещённый символ")
			}
			if strings.HasSuffix(k, "_location") && !model.ValidLocation(s) {
				return nil, model.NewError(422, "VALIDATION_FAILED", "Адрес: три положительных числа через точку, без ведущих нулей")
			}
			if k == "replaces_credential_id" {
				if !model.ValidUUID(s) {
					return nil, model.Invalid("Неверный UUID")
				}
				s = strings.ToLower(s)
			}
			v = s
		}
		in[k] = v
	}
	if _, err := dec.Token(); err != nil {
		return nil, model.Invalid("Некорректный JSON")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, model.Invalid("Лишние данные после JSON")
	}
	for k, required := range allowed {
		if _, ok := in[k]; required && !ok {
			return nil, model.Invalid("Отсутствует обязательное поле")
		}
	}
	return in, nil
}

func (values input) optionalString(key string) *string {
	if value, supplied := values[key]; supplied {
		text := value.(string)
		return &text
	}
	return nil
}

func (values input) optionalBool(key string) *bool {
	if value, supplied := values[key]; supplied {
		active := value.(bool)
		return &active
	}
	return nil
}

func (values input) optionalInt64(key string) *int64 {
	if value, supplied := values[key]; supplied {
		version := value.(int64)
		return &version
	}
	return nil
}
