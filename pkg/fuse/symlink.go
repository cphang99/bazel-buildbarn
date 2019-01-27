package fuse

import (
	"github.com/hanwen/go-fuse/fuse"
)

type symlink struct {
	target string
}

func NewSymlink(target string) Leaf {
	return &symlink{
		target: target,
	}
}

func (i *symlink) GetFUSEDirEntry() fuse.DirEntry {
	return fuse.DirEntry{
		Mode: fuse.S_IFLNK | 0777,
	}
}

func (i *symlink) GetFUSENode() FUSENode {
	return NewSymlinkFUSENode(i.target)
}

func (l *symlink) Unlink() {
}