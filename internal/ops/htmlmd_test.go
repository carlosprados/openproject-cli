package ops

import "testing"

func TestHTMLToMarkdown(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"ckeditor table",
			`Antes<figure class="table op-uc-figure"><table class="op-uc-table"><tbody><tr><td><h3 class="op-uc-h3"><strong>Entregable</strong></h3></td><td><h3><strong>Enlace</strong></h3></td></tr><tr><td><p class="op-uc-p">&nbsp;Alcance&nbsp;</p></td><td><p><a class="op-uc-link" href="https://x.test/d?a=1&amp;b=2">A.docx</a></p><p><a href="https://x.test/e">B | C</a></p></td></tr></tbody></table></figure>Después`,
			"Antes\n\n| **Entregable** | **Enlace** |\n| --- | --- |\n| Alcance | [A.docx](https://x.test/d?a=1&b=2) [B \\| C](https://x.test/e) |\n\nDespués"},
		{"br between list paragraphs",
			"1.  TIPOS\n    \n    <br>\n    \n    Crear tres tipos",
			"1.  TIPOS\n\n    Crear tres tipos"},
		{"details",
			"<details>\n<summary><code>docker-compose.yml</code> (borrador)</summary>\n\n```yaml\na: <b>x</b>\n```\n\n</details>",
			"\n**`docker-compose.yml` (borrador)**\n\n```yaml\na: <b>x</b>\n```\n"},
		{"inline tags and entities",
			`Hola <strong>mundo</strong>, <em>ya</em> &quot;sí&quot;<br>línea <img class="op-uc-image" src="/api/v3/attachments/5/content" alt="foto"> <img>`,
			"Hola **mundo**, *ya* \"sí\"\nlínea ![foto](/api/v3/attachments/5/content) "},
		{"text that looks like tags and code spans stay",
			"Rellena <host> y <Descripción>; `<a href=\"x\">y</a>` y `&nbsp;`",
			"Rellena <host> y <Descripción>; `<a href=\"x\">y</a>` y `&nbsp;`"},
	}
	for _, c := range cases {
		if got := htmlToMarkdown(c.in); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestAttachmentFileName(t *testing.T) {
	if got := attachmentFileName(7, "../My shot (1).png"); got != "7-My-shot--1-.png" {
		t.Fatal(got)
	}
}
