package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

const DefaultBodyCacheKey = "rawBody"

var ErrBodyTooLarge = errors.New("request body exceeds maximum cache size")

type BodyCacheConfig struct {
	// Limit is the maximum number of request-body bytes to cache. A zero value
	// means there is no explicit limit.
	Limit int64

	// ContextKey is the Echo context key used to expose the cached raw body.
	// When empty, DefaultBodyCacheKey is used.
	ContextKey string
}

func BodyCache() echo.MiddlewareFunc {
	return BodyCacheWithConfig(BodyCacheConfig{})
}

func BodyCacheWithLimit(limit int64) echo.MiddlewareFunc {
	return BodyCacheWithConfig(BodyCacheConfig{Limit: limit})
}

func BodyCacheWithConfig(config BodyCacheConfig) echo.MiddlewareFunc {
	if config.ContextKey == "" {
		config.ContextKey = DefaultBodyCacheKey
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			if req == nil || req.Body == nil || req.Body == http.NoBody {
				c.Set(config.ContextKey, []byte(nil))
				return next(c)
			}

			body, err := readBodyWithLimit(req.Body, config.Limit)
			if err != nil {
				if errors.Is(err, ErrBodyTooLarge) {
					return echo.NewHTTPError(http.StatusRequestEntityTooLarge, err.Error())
				}
				return err
			}

			c.Set(config.ContextKey, body)
			req.Body = io.NopCloser(bytes.NewReader(body))

			return next(c)
		}
	}
}

func RestoreCachedBody(c echo.Context, key ...string) error {
	contextKey := DefaultBodyCacheKey
	if len(key) > 0 && key[0] != "" {
		contextKey = key[0]
	}

	body, ok := c.Get(contextKey).([]byte)
	if !ok {
		return errors.New("cached request body is not available")
	}

	c.Request().Body = io.NopCloser(bytes.NewReader(body))
	return nil
}

func readBodyWithLimit(body io.ReadCloser, limit int64) ([]byte, error) {
	defer body.Close()

	if limit <= 0 {
		return io.ReadAll(body)
	}

	limited := io.LimitReader(body, limit+1)
	readBody, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(readBody)) > limit {
		return nil, ErrBodyTooLarge
	}

	return readBody, nil
}
