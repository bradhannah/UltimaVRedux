package atlasterrain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/repositoryfs"
)

const (
	maxPNGBytes          = 80 << 20
	maxMetadataBytes     = 4096
	rgbaChannels         = 4
	artworkColumns       = 32
	privateDirectoryMode = 0700
	privateFileMode      = 0600
)

// Result transfers CPU pixel ownership and nonfatal cache diagnostics to the caller.
type Result struct {
	Image                               image.Image
	Identity, Path                      string
	CacheHit                            bool
	Diagnostic                          error // A failed cache write does not discard usable generated pixels.
	ComposeTime, EncodeTime, DecodeTime time.Duration
}

type cacheMetadata struct {
	Identity string `json:"identity"`
	Policy   string `json:"policy"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	PNGHash  string `json:"pngHash"`
}

// Identity depends only on background pixels' immutable inputs and policy.
func Identity(terrain Terrain, artwork []byte) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(PolicyVersion))
	var cell [2]byte
	for y := range WorldSize {
		for x := range WorldSize {
			binary.LittleEndian.PutUint16(cell[:], terrain[x][y])
			_, _ = digest.Write(cell[:])
		}
	}
	_, _ = digest.Write(artwork)

	return hex.EncodeToString(digest.Sum(nil))
}

// LoadOrGenerate validates immutable inputs even on hits; malformed cache entries
// regenerate. Only the caller decides logging and GPU ownership.
func LoadOrGenerate(root string, terrain Terrain, artwork []byte) (Result, error) {
	var result Result
	result.Identity = Identity(terrain, artwork)
	result.Path = filepath.Join(root, result.Identity+".png")
	normalized, _, err := Normalize(terrain)
	if err != nil {
		return result, err
	}
	start := time.Now()
	if cached, ok := readCache(result.Path, result.Identity); ok {
		result.Image = cached
		result.CacheHit = true
		result.DecodeTime = time.Since(start)

		return result, nil
	}
	start = time.Now()
	result.Image, err = Compose(normalized, artwork)
	result.ComposeTime = time.Since(start)
	if err != nil {
		return result, err
	}
	start = time.Now()
	result.Diagnostic = writeCache(result.Path, result.Identity, result.Image)
	result.EncodeTime = time.Since(start)

	return result, nil
}

// Compose accepts reviewed normalized terrain and never mutates the input sheet.
//
//nolint:varnamelen,gocognit // The bounded source-pixel composition loop keeps shape and position together.
func Compose(terrain Terrain, artwork []byte) (*image.RGBA, error) {
	config, err := png.DecodeConfig(bytes.NewReader(artwork))
	if err != nil {
		return nil, fmt.Errorf("atlas artwork header: %w", err)
	}
	if config.Width != 512 || config.Height != 256 {
		return nil, fmt.Errorf("%w: artwork dimensions %dx%d", ErrUnsupportedSource, config.Width, config.Height)
	}
	sheet, err := png.Decode(bytes.NewReader(artwork))
	if err != nil {
		return nil, fmt.Errorf("atlas artwork: %w", err)
	}
	tiles := make(map[uint16][]byte)
	result := image.NewRGBA(image.Rect(0, 0, ImageSize, ImageSize))
	for y := range WorldSize {
		for x := range WorldSize {
			id := terrain[x][y]
			if id >= 256 || !supportedTile(int(id)) || substrate(id) != id {
				return nil, fmt.Errorf("%w: normalized tile %d", ErrUnsupportedSource, id)
			}
			if tiles[id] == nil {
				pixels := make([]byte, TileSize*TileSize*rgbaChannels)
				for py := range TileSize {
					for px := range TileSize {
						p, ok := color.NRGBAModel.Convert(sheet.At(
							int(id%artworkColumns)*TileSize+px, int(id/artworkColumns)*TileSize+py)).(color.NRGBA)
						if !ok {
							panic("NRGBAModel violated its conversion contract")
						}
						c := cartographicColour(id, p)
						i := (py*TileSize + px) * rgbaChannels
						pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = c.R, c.G, c.B, c.A
					}
				}
				tiles[id] = pixels
			}
			for py := range TileSize {
				offset := result.PixOffset(x*TileSize, y*TileSize+py)
				copy(result.Pix[offset:offset+TileSize*4], tiles[id][py*TileSize*4:(py+1)*TileSize*4])
			}
		}
	}

	return result, nil
}

func readCache(path, identity string) (image.Image, bool) {
	metadataBytes, err := readBounded(path+".json", maxMetadataBytes)
	if err != nil {
		return nil, false
	}
	var metadata cacheMetadata
	if repositoryfs.DecodeStrictJSON(metadataBytes, &metadata) != nil || metadata.Identity != identity ||
		metadata.Policy != PolicyVersion || metadata.Width != ImageSize || metadata.Height != ImageSize {
		return nil, false
	}
	encoded, err := readBounded(path, maxPNGBytes)
	if err != nil {
		return nil, false
	}
	hash := sha256.Sum256(encoded)
	if metadata.PNGHash != hex.EncodeToString(hash[:]) {
		return nil, false
	}
	config, err := png.DecodeConfig(bytes.NewReader(encoded))
	if err != nil || config.Width != ImageSize || config.Height != ImageSize {
		return nil, false
	}
	decoded, err := png.Decode(bytes.NewReader(encoded))

	return decoded, err == nil
}

//nolint:nonamedreturns,varnamelen // Deferred close failures augment the read result; f is the bounded input file.
func readBounded(path string, limit int64) (content []byte, err error) {
	//nolint:gosec // Paths are caller-selected disposable cache entries, not remote input.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open atlas cache: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close atlas cache: %w", closeErr))
		}
	}()
	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat atlas cache: %w", err)
	}
	if stat.Size() > limit {
		return nil, fmt.Errorf("%w: cache exceeds %d bytes", ErrUnsupportedSource, limit)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read atlas cache: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: cache exceeds %d bytes", ErrUnsupportedSource, limit)
	}

	return data, nil
}

func writeCache(path, identity string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), privateDirectoryMode); err != nil {
		return fmt.Errorf("create atlas cache: %w", err)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return fmt.Errorf("encode atlas: %w", err)
	}
	hash := sha256.Sum256(encoded.Bytes())
	metadata, err := json.Marshal(cacheMetadata{
		identity, PolicyVersion, ImageSize, ImageSize, hex.EncodeToString(hash[:]),
	})
	if err != nil {
		return fmt.Errorf("encode atlas identity: %w", err)
	}
	if err = publish(path, encoded.Bytes()); err != nil {
		return err
	}

	return publish(path+".json", metadata)
}

func publish(path string, data []byte) error {
	err := repositoryfs.PublishTemporaryFile(filepath.Dir(path), ".atlas-*", data, privateFileMode,
		func(temporary string) error {
			if renameErr := os.Rename(temporary, path); renameErr != nil {
				return fmt.Errorf("rename atlas cache: %w", renameErr)
			}

			return nil
		})
	if err != nil {
		return fmt.Errorf("publish atlas cache: %w", err)
	}
	// The shared helper skips Windows, which cannot sync a directory handle.
	if err := repositoryfs.SyncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync atlas cache directory: %w", err)
	}

	return nil
}
