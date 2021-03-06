package impl

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"go.dedis.ch/cs438/peer"
)

func (n *node) CreateAndPublishFolderRecord(path string, folderName string, privateKey *rsa.PrivateKey, sequence, ttl uint) (string, error) {
	// TODO should probably always use the same private key in this method
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return "", err
	}
	pointerRecordHashes := make([]string, 0)
	for _, file := range files {
		filePath := filepath.Join(path, file.Name())
		privateKey2, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return "", err
		}
		if file.IsDir() {
			recordHash, err := n.CreateAndPublishFolderRecord(filePath, file.Name(), privateKey2, sequence, ttl)
			if err != nil {
				return "", err
			}
			pointerRecordHashes = append(pointerRecordHashes, recordHash)
		} else {
			fileR, err := os.Open(filePath)
			if err != nil {
				return "", err
			}
			mh, err := n.UploadDHT(bufio.NewReader(fileR))
			if err != nil {
				return "", err
			}
			record, err := n.CreatePointerRecord(privateKey2, file.Name(), mh, sequence, ttl)
			if err != nil {
				return "", err
			}
			recordHash, err := n.PublishPointerRecord(record)
			if err != nil {
				return "", err
			}
			pointerRecordHashes = append(pointerRecordHashes, recordHash)
		}
	}
	record, err := n.CreateFolderPointerRecord(privateKey, folderName, pointerRecordHashes, sequence, ttl)
	if err != nil {
		return "", err
	}
	recordHash, err := n.PublishPointerRecord(record)
	if err != nil {
		return "", err
	}
	return recordHash, err
}

func (n *node) ReconstructFolderFromRecord(basePath string, record peer.PointerRecord, keep bool) (string, error) {
	if !n.IsFolderRecord(record) {
		return "", fmt.Errorf("record should be a folder to reconstruct it")
	}
	err := os.RemoveAll(filepath.Join(basePath, record.Name))
	if err != nil {
		return "", err
	}
	err = os.Mkdir(filepath.Join(basePath, record.Name), 0777)
	if err != nil {
		return "", err
	}
	for _, f := range record.Links {
		fetchedRecord, ok := n.FetchPointerRecord(f)
		if !ok {
			return "", fmt.Errorf("record not found: " + f)
		}
		if n.IsFolderRecord(fetchedRecord) {
			n.ReconstructFolderFromRecord(filepath.Join(basePath, record.Name), fetchedRecord, keep)
		} else {
			res, err := n.DownloadDHT(fetchedRecord.Value, keep)
			if err != nil {
				return "", err
			}
			err = ioutil.WriteFile(filepath.Join(basePath, record.Name, fetchedRecord.Name), res, 0666)
			if err != nil {
				return "", err
			}
		}
	}
	return basePath + record.Name, nil
}
