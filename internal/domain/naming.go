package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var placeholderPattern = regexp.MustCompile(`\{[A-Za-z]+\}`)

func ValidateNamingRule(rule string) error {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return Invalid("namingRule", "不能为空")
	}
	if strings.Contains(rule, "/") || strings.Contains(rule, "\\") {
		return Invalid("namingRule", "不得包含目录分隔符")
	}
	if !strings.Contains(rule, "{carrierCode}") {
		return Invalid("namingRule", "必须包含 {carrierCode} 占位符")
	}
	allowed := map[string]bool{"{carrierCode}": true, "{sequence}": true, "{collectionRef}": true}
	for _, placeholder := range placeholderPattern.FindAllString(rule, -1) {
		if !allowed[placeholder] {
			return Invalid("namingRule", "包含不支持的占位符 "+placeholder)
		}
	}
	if !strings.HasSuffix(strings.ToLower(rule), ".wav") {
		return Invalid("namingRule", "数字母版命名必须以 .wav 结尾")
	}
	return nil
}

func RenderCaptureFilename(rule, carrierCode, collectionRef string, sequence int) (string, error) {
	if err := ValidateNamingRule(rule); err != nil {
		return "", err
	}
	carrier := safeFilenamePart(carrierCode)
	collection := safeFilenamePart(collectionRef)
	if carrier == "" {
		return "", Invalid("carrierCode", "无法生成安全文件名")
	}
	values := map[string]string{"{carrierCode}": carrier, "{collectionRef}": collection, "{sequence}": strconv.Itoa(sequence)}
	name := rule
	for placeholder, value := range values {
		name = strings.ReplaceAll(name, placeholder, value)
	}
	if placeholderPattern.MatchString(name) {
		return "", fmt.Errorf("%w: 命名规则仍有未解析占位符", ErrInvalid)
	}
	if len(name) > 180 {
		return "", Invalid("namingRule", "生成的文件名超过 180 个字符")
	}
	return name, nil
}

func safeFilenamePart(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastSeparator := false
	for _, r := range value {
		allowed := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
		if allowed {
			b.WriteRune(r)
			lastSeparator = false
			continue
		}
		if !lastSeparator && b.Len() > 0 {
			b.WriteByte('_')
			lastSeparator = true
		}
	}
	return strings.Trim(b.String(), "_")
}
