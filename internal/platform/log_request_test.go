package platform

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONDecoderRejectsTrailingAndOversizedInput(t *testing.T) {
	for _, body := range []string{"{}{}", "{}" + strings.Repeat(" ", 1<<20)} {
		request := httptest.NewRequest("POST", "/", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		var decoded map[string]any
		if err := decode(request, &decoded); err == nil {
			t.Fatal("unsafe JSON request accepted")
		}
	}
}
