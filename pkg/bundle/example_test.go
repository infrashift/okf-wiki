package bundle_test

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"log"
	"testing/fstest"

	"github.com/infrashift/okf-wiki/pkg/bundle"
)

// A consumer that receives a bundle as a tar archive extracts it into memory,
// roots the FS at .okfw, loads it once and searches the result.
func Example() {
	archive := tarOf(map[string]string{
		".okfw/index.md": "# Listing\n",
		".okfw/decisions/adr-1.md": "---\ntitle: Refresh tokens in the gateway\ntype: ADR\n" +
			"tags: [auth]\ndescription: The gateway refreshes OIDC tokens.\n---\n" +
			"The gateway refreshes a token one minute before it expires.\n",
		".okfw/specs/sessions.md": "---\ntitle: Sessions\ntype: Spec\ntags: []\n---\n" +
			"A session outlives its token; see the refresh decision.\n",
	})

	// The extraction is the consumer's: it must bound the archive and refuse
	// links and paths outside the tree. Only regular files are kept here.
	mem := fstest.MapFS{}
	tr := tar.NewReader(bytes.NewReader(archive))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		if h.Typeflag != tar.TypeReg || !fs.ValidPath(h.Name) {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, bundle.DefaultLimits.MaxFileBytes+1))
		if err != nil {
			log.Fatal(err)
		}
		mem[h.Name] = &fstest.MapFile{Data: data}
	}

	root, err := fs.Sub(mem, ".okfw")
	if err != nil {
		log.Fatal(err)
	}
	concepts, err := bundle.Load(root, bundle.DefaultLimits)
	if err != nil {
		log.Fatal(err) // errors.Is(err, bundle.ErrFileTooLarge), ...
	}
	for _, h := range bundle.Search(concepts, "refresh token", 25) {
		fmt.Printf("%s %q (%s)\n", h.Concept.ID, h.Concept.Title, h.Concept.Type)
	}
	// Output:
	// decisions/adr-1 "Refresh tokens in the gateway" (ADR)
	// specs/sessions "Sessions" (Spec)
}

func tarOf(files map[string]string) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			log.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			log.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}
