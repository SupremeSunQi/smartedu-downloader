package logging

import "regexp"

var (
	authorizationHeader = regexp.MustCompile(`(?i)(authorization\s*:\s*)[^\r\n]+`)
	secretQuery         = regexp.MustCompile(`(?i)([?&](?:accessToken|access_token|token|password|authorization|mac_key)=)[^&\s"'}]+`)
	secretField         = regexp.MustCompile(`(?i)(["']?(?:accessToken|access_token|token|password|authorization|mac_key)["']?\s*[:=]\s*["']?)[^"',}\s&]+`)
)

func Redact(value string) string {
	value = authorizationHeader.ReplaceAllString(value, `${1}[REDACTED]`)
	value = secretQuery.ReplaceAllString(value, `${1}[REDACTED]`)
	return secretField.ReplaceAllString(value, `${1}[REDACTED]`)
}
