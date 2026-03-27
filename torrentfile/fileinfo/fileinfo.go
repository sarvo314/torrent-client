package fileinfo

import (
	"fmt"
	"os"
)

type FileInfo struct {
	GlobalPath  string
	Length      int
	GlobalStart int
	GlobalEnd   int
}

func NewFileInfo(globalPath string, length int) *FileInfo {
	return &FileInfo{
		GlobalPath:  globalPath,
		Length:      length,
		GlobalStart: 0,
		GlobalEnd:   length,
	}
}

func (f *FileInfo) SetGlobalStart(start int) {
	f.GlobalStart = start
}

func (f *FileInfo) SetGlobalEnd(end int) {
	f.GlobalEnd = end
}
func (f *FileInfo) WriteToFile(buf []byte, offset int64) {
	file, err := os.OpenFile(f.GlobalPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		fmt.Println("Stat error:", err)
		return
	}

	// Ensure file is large enough
	if info.Size() < int64(f.Length) {
		err = file.Truncate(int64(f.Length))
		if err != nil {
			fmt.Println("Truncate error:", err)
			return
		}
	}

	// Write at offset
	_, err = file.WriteAt(buf, offset)
	if err != nil {
		fmt.Println("WriteAt error:", err)
		return
	}
}
