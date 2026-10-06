package handler

import (
	"battery-storage-pandora/internal/model"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"
)

func decodeRequest(w http.ResponseWriter, r *http.Request, body any, allowed string) error {
	if err := noQuery(r); err != nil {
		return err
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return model.NewError(415, "UNSUPPORTED_MEDIA_TYPE", "Ожидается application/json")
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return model.NewError(413, "PAYLOAD_TOO_LARGE", "Максимальный размер тела — 64 KiB")
		}
		return model.Invalid("Не удалось прочитать тело запроса")
	}
	if !utf8.Valid(data) {
		return model.Invalid("Ожидается UTF-8 JSON")
	}
	if err := checkJSONObject(data, allowed); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(body); err != nil {
		return model.Invalid("Неверное поле или тип значения JSON")
	}
	return nil
}

// The standard decoder accepts duplicate fields, null and case-insensitive names.
// Check the top-level fields first, then decode into an ordinary request struct.
func checkJSONObject(data []byte, allowed string) error {
	fields := make(map[string]bool)
	for _, field := range strings.Fields(allowed) {
		fields[field] = true
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return model.Invalid("Ожидается JSON-объект")
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return model.Invalid("Некорректный JSON")
		}
		field, ok := token.(string)
		if !ok || !fields[field] {
			return model.Invalid("Неизвестное поле JSON")
		}
		if seen[field] {
			return model.Invalid("Повторяющееся поле JSON")
		}
		seen[field] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return model.Invalid("Некорректный JSON")
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return model.Invalid("Значение null не поддерживается")
		}
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') {
		return model.Invalid("Некорректный JSON")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return model.Invalid("Лишние данные после JSON")
	}
	return nil
}

func requiredText(values ...*string) error {
	for _, value := range values {
		if value == nil {
			return model.Invalid("Отсутствует обязательное поле")
		}
	}
	return optionalText(values...)
}

func optionalText(values ...*string) error {
	for _, value := range values {
		if value != nil && (strings.TrimSpace(*value) == "" || strings.ContainsRune(*value, 0)) {
			return model.NewError(422, "VALIDATION_FAILED", "Пустая строка или запрещённый символ")
		}
	}
	return nil
}

func validateLocations(values ...*string) error {
	for _, value := range values {
		if value != nil && !model.ValidLocation(*value) {
			return model.NewError(422, "VALIDATION_FAILED", "Адрес: три положительных числа через точку, без ведущих нулей")
		}
	}
	return nil
}
