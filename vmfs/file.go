package vmfs

import (
	"fmt"
	"time"

	"github.com/koding/fuseklient/transport"

	"bazil.org/fuse"

	"golang.org/x/net/context"
)

type File struct {
	*Node

	// Parent is pointer to Dir that holds this Dir. Each File/Dir has single
	// parent; a parent can have multiple children.
	Parent *Dir
}

// ReadAll returns the entire file. Required by Fuse.
func (f *File) ReadAll(ctx context.Context) ([]byte, error) {
	defer debug(time.Now(), "File="+f.Name)

	req := struct{ Path string }{f.RemotePath}
	res := transport.FsReadFileRes{}
	if err := f.Trip("fs.readFile", req, &res); err != nil {
		return []byte{}, err
	}

	return res.Content, nil
}

// Write rewrites all data to file. Required by Fuse.
func (f *File) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	defer debug(time.Now(), "File="+f.Name, fmt.Sprintf("OldContentLength=%v", f.attr.Size), fmt.Sprintf("NewContentLength=%v", len(req.Data)))

	if err := f.write(req.Data); err != nil {
		return err
	}

	f.RLock()
	resp.Size = int(f.attr.Size)
	f.RUnlock()

	return nil
}

func (f *File) Fsync(ctx context.Context, req *fuse.FsyncRequest) error {
	defer debug(time.Now(), "File="+f.Name, fmt.Sprintf("OldContentLength=%v", f.attr.Size))
	return nil
}

// write tells Transport to overwrite file on user VM with contents.
func (f *File) write(data []byte) error {
	f.Lock()
	defer f.Unlock()

	treq := struct {
		Path    string
		Content []byte
	}{
		Path:    f.RemotePath,
		Content: data,
	}
	var tres int
	if err := f.Transport.Trip("fs.writeFile", treq, &tres); err != nil {
		return err
	}

	f.attr.Size = uint64(tres)
	f.attr.Mtime = time.Now()

	return f.Parent.invalidateCache(f.Name)
}
