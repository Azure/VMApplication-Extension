package downloader

type IDownloader interface {
	Download(uri, downloadPath string) error
}
