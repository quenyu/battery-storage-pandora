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

type requestBody interface {
	validate() error
}

func decodeCommand(w http.ResponseWriter, r *http.Request, body requestBody) error {
	if err := normalizePath(r); err != nil {
		return err
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !model.ValidUUID(keys[0]) {
		return model.Invalid("Нужен UUID Idempotency-Key")
	}
	r.Header.Set("Idempotency-Key", strings.ToLower(keys[0]))
	return decodeRequest(w, r, body)
}

func decodeRequest(w http.ResponseWriter, r *http.Request, body requestBody) error {
	if err := noQuery(r); err != nil {
		return err
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return model.NewError(415, "UNSUPPORTED_MEDIA_TYPE", "Ожидается application/json")
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		return model.NewError(413, "PAYLOAD_TOO_LARGE", "Максимальный размер тела — 64 KiB")
	}
	if !utf8.Valid(data) {
		return model.Invalid("Ожидается UTF-8 JSON")
	}
	fields, err := checkJSONObject(data)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(body); err != nil {
		return model.Invalid("Неверное поле или тип значения JSON")
	}
	// encoding/json also accepts case-insensitive names; the API uses exact JSON tags.
	declaredFields, err := jsonFields(body)
	if err != nil {
		return err
	}
	for field := range fields {
		if _, exists := declaredFields[field]; !exists {
			return model.Invalid("Неизвестное поле")
		}
	}
	return body.validate()
}

// The standard decoder allows duplicate fields and null. Check these before decoding.
func checkJSONObject(data []byte) (map[string]bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, model.Invalid("Ожидается JSON-объект")
	}
	fields := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, model.Invalid("Некорректный JSON")
		}
		field, ok := token.(string)
		if !ok || fields[field] {
			return nil, model.Invalid("Повторяющееся поле JSON")
		}
		fields[field] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, model.Invalid("Неверное значение поля")
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, model.Invalid("Некорректный JSON")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, model.Invalid("Лишние данные после JSON")
	}
	return fields, nil
}

func jsonFields(value any) (map[string]json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	err = json.Unmarshal(data, &fields)
	return fields, err
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
	if err := optionalText(values...); err != nil {
		return err
	}
	for _, value := range values {
		if value != nil && !model.ValidLocation(*value) {
			return model.NewError(422, "VALIDATION_FAILED", "Адрес: три положительных числа через точку, без ведущих нулей")
		}
	}
	return nil
}

func validateVersion(version *int64) error {
	if version != nil && *version < 1 {
		return model.NewError(422, "VALIDATION_FAILED", "Версия должна быть положительной")
	}
	return nil
}
