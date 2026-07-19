package v2

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/gin-gonic/gin"
)

func writePackageResponse(c *gin.Context, route Route, response pluginhost.DispatchOutput) error {
	if response.StatusCode < http.StatusContinue || response.StatusCode > 599 {
		return errors.New("package host returned an invalid response status")
	}
	hopByHop := packageHopByHopHeaders(response.Headers)
	for _, header := range response.Headers {
		if shouldForwardResponseHeader(route, header, hopByHop) {
			c.Writer.Header().Add(header.Name, header.Value)
		}
	}
	status := int(response.StatusCode)
	if status == http.StatusNoContent || status == http.StatusNotModified || len(response.Body) == 0 {
		c.Status(status)
		return nil
	}
	switch route.Envelope {
	case EnvelopeRaw:
		c.Data(status, c.Writer.Header().Get("Content-Type"), response.Body)
	case EnvelopeData:
		if !json.Valid(response.Body) {
			return errors.New("package host returned a non-JSON data envelope")
		}
		if c.Writer.Header().Get("Content-Type") == "" {
			c.Header("Content-Type", "application/json; charset=utf-8")
		}
		if validPanelResponse(response.Body) {
			c.Data(status, c.Writer.Header().Get("Content-Type"), response.Body)
			return nil
		}
		c.JSON(status, gin.H{"data": json.RawMessage(response.Body)})
	case EnvelopePanel:
		if !validPanelResponse(response.Body) {
			return errors.New("package host returned an invalid panel envelope")
		}
		if c.Writer.Header().Get("Content-Type") == "" {
			c.Header("Content-Type", "application/json; charset=utf-8")
		}
		c.Data(status, c.Writer.Header().Get("Content-Type"), response.Body)
	default:
		return errors.New("package route has an unsupported HTTP envelope")
	}
	return nil
}

func validResponseHeader(name, value string) bool {
	if name == "" || value == "" || len(name) > 256 || len(value) > 8192 {
		return false
	}
	for _, character := range name {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-') {
			return false
		}
	}
	for _, character := range value {
		if character == '\r' || character == '\n' || character == 0 {
			return false
		}
	}
	return true
}

func packageHopByHopHeaders(headers []pluginhost.Header) map[string]struct{} {
	blocked := map[string]struct{}{
		"connection": {}, "keep-alive": {}, "proxy-connection": {}, "te": {},
		"trailer": {}, "transfer-encoding": {}, "upgrade": {},
	}
	for _, header := range headers {
		if !strings.EqualFold(header.Name, "Connection") {
			continue
		}
		for _, token := range strings.Split(header.Value, ",") {
			if token = strings.TrimSpace(token); token != "" {
				blocked[strings.ToLower(token)] = struct{}{}
			}
		}
	}
	return blocked
}

func shouldForwardResponseHeader(route Route, header pluginhost.Header, hopByHop map[string]struct{}) bool {
	if !validResponseHeader(header.Name, header.Value) {
		return false
	}
	name := strings.ToLower(header.Name)
	if _, blocked := hopByHop[name]; blocked || name == "content-length" {
		return false
	}
	if route.Envelope == EnvelopeData || route.Envelope == EnvelopePanel {
		switch name {
		case "content-type", "content-encoding", "content-range":
			return false
		}
	}
	return true
}

func validPanelResponse(body []byte) bool {
	var panel struct {
		Code *int            `json:"code"`
		Msg  *string         `json:"msg"`
		TS   *int64          `json:"ts"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &panel); err != nil {
		return false
	}
	return panel.Code != nil && panel.Msg != nil && panel.TS != nil && panel.Data != nil && json.Valid(panel.Data)
}
