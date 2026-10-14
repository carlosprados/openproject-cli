package ops

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestTextShowsTimestampsInLocalTime(t *testing.T) {
	saved := time.Local
	time.Local = time.FixedZone("CEST", 2*3600)
	defer func() { time.Local = saved }()

	var buf bytes.Buffer
	rec := map[string]any{"createdAt": "2026-10-14T16:20:25.688Z", "dueDate": "2026-10-21", "subject": "2026-10-14T16:20:25Z is not a key"}
	if err := renderRecord(&buf, rec); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"2026-10-14 18:20", "2026-10-21"} {
		if !strings.Contains(out, want) {
			t.Errorf("record lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "16:20:25.688Z") {
		t.Errorf("raw UTC timestamp left in text:\n%s", out)
	}

	buf.Reset()
	l := NewList([]map[string]any{{"id": 1, "createdAt": "2026-10-14T16:20:25Z"}}, 1)
	if err := RenderText(&buf, ByCmd("notification list"), l); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "2026-10-14 18:20") {
		t.Errorf("notification table not in local time:\n%s", buf.String())
	}
}
