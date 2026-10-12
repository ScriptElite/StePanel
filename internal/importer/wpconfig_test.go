package importer

import (
	"errors"
	"strings"
	"testing"
)

func TestReadAndReplacePHPDefine(t *testing.T) {
	content := "<?php\n// define('DB_NAME', 'commented');\ndefine( 'DB_NAME' , \"shop_db\" );\ndefine('DB_USER', getenv('DB_USER'));\n"
	value, found, err := ReadPHPDefine(content, "DB_NAME")
	if err != nil || !found || value != "shop_db" {
		t.Fatalf("ReadPHPDefine(DB_NAME) = %q, %v, %v", value, found, err)
	}
	if _, found, err := ReadPHPDefine(content, "DB_USER"); !found || !errors.Is(err, ErrFunctionBackedDefine) {
		t.Fatalf("function-backed DB_USER = found %v, err %v", found, err)
	}
	if _, found, err := ReadPHPDefine(content, "DB_HOST"); found || err != nil {
		t.Fatalf("missing DB_HOST = found %v, err %v", found, err)
	}
	updated, found, err := ReplacePHPDefine(content, "DB_NAME", `new"db`)
	if err != nil || !found || !strings.Contains(updated, `"new\"db"`) || !strings.Contains(updated, "'commented'") {
		t.Fatalf("ReplacePHPDefine = %q, %v, %v", updated, found, err)
	}
	if same, found, err := ReplacePHPDefine(content, "DB_NAME", "shop_db"); err != nil || !found || same != content {
		t.Fatalf("unchanged replacement = found %v, err %v", found, err)
	}
	if _, found, err := ReplacePHPDefine(content, "DB_USER", "x"); !found || !errors.Is(err, ErrFunctionBackedDefine) {
		t.Fatalf("function-backed replacement = found %v, err %v", found, err)
	}
	if same, found, err := ReplacePHPDefine(content, "DB_HOST", "x"); found || err != nil || same != content {
		t.Fatalf("missing replacement = found %v, err %v", found, err)
	}
	if literal, err := EncodePHPString(`a$b"c`); err != nil || literal != `"a\$b\"c"` {
		t.Fatalf("EncodePHPString = %q, %v", literal, err)
	}
}
