package importer

import "errors"

// ErrFunctionBackedDefine reports a define() whose value is computed by a
// function such as getenv(). Such a value cannot be read or replaced with a
// literal without changing the configuration's meaning.
var ErrFunctionBackedDefine = errors.New("define value is computed by a function call")

// ReadPHPDefine returns the literal string value of the single active
// define(key, ...) in content. found is false when the constant is not
// defined. A function-backed value returns ErrFunctionBackedDefine; an
// ambiguous or malformed definition returns another error.
func ReadPHPDefine(content, key string) (value string, found bool, err error) {
	definition, found, err := findPHPDefine(content, key)
	if err != nil || !found {
		return "", false, err
	}
	if definition.skipped {
		return "", true, ErrFunctionBackedDefine
	}
	return unescapePHPKey(content[definition.valueStart+1 : definition.valueEnd-1]), true, nil
}

// ReplacePHPDefine replaces the literal value of the single active
// define(key, ...) in content. found is false, and content is returned
// unchanged, when the constant is not defined. A function-backed value
// returns ErrFunctionBackedDefine instead of being overwritten.
func ReplacePHPDefine(content, key, value string) (updated string, found bool, err error) {
	updated, changed, skipped, err := replaceDefineValue(content, key, "", value)
	if err != nil {
		return content, false, err
	}
	if skipped {
		return content, true, ErrFunctionBackedDefine
	}
	if changed {
		return updated, true, nil
	}
	_, found, err = findPHPDefine(content, key)
	return content, found, err
}

// EncodePHPString returns value as a double-quoted PHP string literal.
func EncodePHPString(value string) (string, error) {
	encoded, err := encodePHPDoubleQuotedString(value)
	if err != nil {
		return "", err
	}
	return `"` + encoded + `"`, nil
}
