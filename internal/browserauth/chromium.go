package browserauth

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func (reader *Reader) readProfile(ctx context.Context, path string) ([]string, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		memory, err := reader.cloneLevelDB(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}
		database, err := leveldb.Open(memory, nil)
		if err != nil {
			memory.Close()
			lastErr = fmt.Errorf("open browser storage snapshot: %w", err)
			continue
		}
		tokens, scanErr := readSmartEduTokens(ctx, database, reader.options.Now())
		closeErr := database.Close()
		if scanErr != nil {
			if errors.Is(scanErr, ErrExpired) || errors.Is(scanErr, ErrIncompatibleStorage) || errors.Is(scanErr, context.Canceled) || errors.Is(scanErr, context.DeadlineExceeded) {
				return nil, scanErr
			}
			lastErr = fmt.Errorf("read browser session records: %w", scanErr)
			continue
		}
		if closeErr != nil {
			lastErr = fmt.Errorf("close browser storage snapshot: %w", closeErr)
			continue
		}
		return tokens, nil
	}
	if lastErr == nil {
		lastErr = ErrStorage
	}
	return nil, lastErr
}

func readSmartEduTokens(ctx context.Context, database *leveldb.DB, now time.Time) ([]string, error) {
	iterator := database.NewIterator(util.BytesPrefix([]byte("_https://")), nil)
	defer iterator.Release()
	var tokens []string
	var expired, incompatible bool
	for iterator.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !isSmartEduTokenRecord(iterator.Key()) {
			continue
		}
		token, err := decodeSDKValue(iterator.Value(), now)
		switch {
		case errors.Is(err, ErrExpired):
			expired = true
		case err != nil:
			incompatible = true
		default:
			tokens = append(tokens, token)
		}
	}
	if err := iterator.Error(); err != nil {
		return nil, err
	}
	if len(tokens) > 0 {
		return tokens, nil
	}
	if incompatible {
		return nil, ErrIncompatibleStorage
	}
	if expired {
		return nil, ErrExpired
	}
	return nil, nil
}

func isSmartEduTokenRecord(recordKey []byte) bool {
	if len(recordKey) == 0 || recordKey[0] != '_' {
		return false
	}
	origin, key, found := bytes.Cut(recordKey[1:], []byte{0, 1})
	if !found || !bytes.HasPrefix(key, []byte("ND_UC_AUTH-")) || !bytes.HasSuffix(key, []byte("&token")) {
		return false
	}
	parsed, err := url.Parse(string(origin))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "smartedu.cn" || strings.HasSuffix(host, ".smartedu.cn")
}

func (reader *Reader) cloneLevelDB(ctx context.Context, path string) (storage.Storage, error) {
	rootInfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: profile storage path is not a directory", ErrStorage)
	}
	current, err := os.ReadFile(filepath.Join(path, "CURRENT"))
	if err != nil {
		return nil, err
	}
	manifest, ok := parseLevelDBFile(strings.TrimSpace(string(current)))
	if !ok || manifest.Type != storage.TypeManifest {
		return nil, fmt.Errorf("%w: current manifest is invalid", ErrIncompatibleStorage)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	type sourceFile struct {
		path string
		info os.FileInfo
		fd   storage.FileDesc
	}
	files := make([]sourceFile, 0, len(entries))
	var total int64
	for _, entry := range entries {
		fd, recognized := parseLevelDBFile(entry.Name())
		if !recognized || fd.Type == storage.TypeTemp {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: browser storage contains an unsafe file", ErrStorage)
		}
		total += info.Size()
		if len(files)+1 > reader.options.MaxSnapshotFiles || total > reader.options.MaxSnapshotBytes {
			return nil, fmt.Errorf("%w: browser storage snapshot exceeds its limit", ErrStorage)
		}
		files = append(files, sourceFile{path: filepath.Join(path, entry.Name()), info: info, fd: fd})
	}
	memory := storage.NewMemStorage()
	for _, source := range files {
		if err := ctx.Err(); err != nil {
			memory.Close()
			return nil, err
		}
		if err := copyIntoMemory(ctx, memory, source); err != nil {
			memory.Close()
			return nil, err
		}
	}
	if err := memory.SetMeta(manifest); err != nil {
		memory.Close()
		return nil, err
	}
	return memory, nil
}

func copyIntoMemory(ctx context.Context, memory storage.Storage, source struct {
	path string
	info os.FileInfo
	fd   storage.FileDesc
}) error {
	input, err := os.Open(source.path)
	if err != nil {
		return err
	}
	output, err := memory.Create(source.fd)
	if err != nil {
		input.Close()
		return err
	}
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			input.Close()
			output.Close()
			return err
		}
		count, readErr := input.Read(buffer)
		if count > 0 {
			if _, err := output.Write(buffer[:count]); err != nil {
				input.Close()
				output.Close()
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			input.Close()
			output.Close()
			return readErr
		}
	}
	if err := input.Close(); err != nil {
		output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	current, err := os.Stat(source.path)
	if err != nil {
		return err
	}
	if current.Size() != source.info.Size() || !current.ModTime().Equal(source.info.ModTime()) {
		return fmt.Errorf("%w: browser storage changed during detection", ErrStorage)
	}
	return nil
}

func parseLevelDBFile(name string) (storage.FileDesc, bool) {
	if strings.HasPrefix(name, "MANIFEST-") {
		number, err := strconv.ParseInt(strings.TrimPrefix(name, "MANIFEST-"), 10, 64)
		return storage.FileDesc{Type: storage.TypeManifest, Num: number}, err == nil
	}
	extension := filepath.Ext(name)
	number, err := strconv.ParseInt(strings.TrimSuffix(name, extension), 10, 64)
	if err != nil {
		return storage.FileDesc{}, false
	}
	switch extension {
	case ".log":
		return storage.FileDesc{Type: storage.TypeJournal, Num: number}, true
	case ".ldb", ".sst":
		return storage.FileDesc{Type: storage.TypeTable, Num: number}, true
	case ".tmp":
		return storage.FileDesc{Type: storage.TypeTemp, Num: number}, true
	default:
		return storage.FileDesc{}, false
	}
}

func decodeSDKValue(value []byte, now time.Time) (string, error) {
	if len(value) == 0 || len(value) > maxStoredValueBytes {
		return "", ErrIncompatibleStorage
	}
	text, err := decodeChromiumString(value)
	if err != nil {
		return "", err
	}
	var wrapper struct {
		Value  string `json:"value"`
		Expire int64  `json:"expire"`
	}
	if err := json.Unmarshal([]byte(text), &wrapper); err != nil || wrapper.Value == "" {
		return "", ErrIncompatibleStorage
	}
	if wrapper.Expire > 0 && wrapper.Expire <= now.UnixMilli() {
		return "", ErrExpired
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(wrapper.Value), &token); err != nil {
		return "", ErrIncompatibleStorage
	}
	token.AccessToken = strings.TrimSpace(token.AccessToken)
	if token.AccessToken == "" || len(token.AccessToken) > maxStoredValueBytes {
		return "", ErrIncompatibleStorage
	}
	return token.AccessToken, nil
}

func decodeChromiumString(value []byte) (string, error) {
	switch value[0] {
	case 1:
		return string(value[1:]), nil
	case 0:
		encoded := value[1:]
		if len(encoded)%2 != 0 {
			return "", ErrIncompatibleStorage
		}
		units := make([]uint16, len(encoded)/2)
		for index := range units {
			units[index] = binary.LittleEndian.Uint16(encoded[index*2 : index*2+2])
		}
		return string(utf16.Decode(units)), nil
	default:
		return "", ErrIncompatibleStorage
	}
}
