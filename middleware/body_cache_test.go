package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

type jsonPayload struct {
	Foo string `json:"foo" xml:"foo" form:"foo"`
}

func TestBodyCacheRestoresJSONAfterBind(t *testing.T) {
	body := `{"foo":"bar"}`

	gotBody, status := bindThenReadBody(t, "application/json", body)

	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
	if gotBody != body {
		t.Fatalf("expected downstream body %q, got %q", body, gotBody)
	}
}

func TestBodyCacheRestoresXMLAfterBind(t *testing.T) {
	body := `<jsonPayload><foo>bar</foo></jsonPayload>`

	gotBody, status := bindThenReadBody(t, "application/xml", body)

	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
	if gotBody != body {
		t.Fatalf("expected downstream body %q, got %q", body, gotBody)
	}
}

func TestBodyCacheRestoresFormAfterBind(t *testing.T) {
	values := url.Values{"foo": []string{"bar"}}
	body := values.Encode()

	gotBody, status := bindThenReadBody(t, echo.MIMEApplicationForm, body)

	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
	if gotBody != body {
		t.Fatalf("expected downstream body %q, got %q", body, gotBody)
	}
}

func TestBodyCacheHandlesEmptyBody(t *testing.T) {
	e := echo.New()
	e.Use(BodyCache())
	e.POST("/", func(c echo.Context) error {
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.String(http.StatusOK, string(body))
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != "" {
		t.Fatalf("expected empty body, got %q", rec.Body.String())
	}
}

func TestBodyCacheRejectsBodyOverLimit(t *testing.T) {
	e := echo.New()
	e.Use(BodyCacheWithLimit(3))
	e.POST("/", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("abcd"))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413, got %d", rec.Code)
	}
}

func TestRestoreCachedBodyAllowsMultipleReads(t *testing.T) {
	e := echo.New()
	e.Use(BodyCache())
	e.POST("/", func(c echo.Context) error {
		firstRead, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		if err := RestoreCachedBody(c); err != nil {
			return err
		}
		secondRead, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		if !bytes.Equal(firstRead, secondRead) {
			t.Fatalf("expected restored body %q, got %q", firstRead, secondRead)
		}
		return c.String(http.StatusOK, string(secondRead))
	})

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"foo":"bar"}`))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func bindThenReadBody(t *testing.T, contentType string, body string) (string, int) {
	t.Helper()

	e := echo.New()
	e.Use(BodyCache())
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			var payload jsonPayload
			if err := c.Bind(&payload); err != nil {
				return err
			}
			if payload.Foo != "bar" {
				encoded, _ := json.Marshal(payload)
				return echo.NewHTTPError(http.StatusBadRequest, string(encoded))
			}
			if err := RestoreCachedBody(c); err != nil {
				return err
			}
			return next(c)
		}
	})
	e.POST("/", func(c echo.Context) error {
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.String(http.StatusOK, string(body))
	})

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	return rec.Body.String(), rec.Code
}
