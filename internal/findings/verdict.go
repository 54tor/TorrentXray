package findings

func (f Findings) HasProblems() bool {
	if f.Torrent != nil && !f.Torrent.ContentMatching() {
		return true
	}
	for _, check := range f.Checks {
		if check.HasProblems() {
			return true
		}
	}
	return false
}

func (c FileCheck) HasProblems() bool {
	ffmpegReportedErrors := c.FfmpegRan && (c.FfmpegExitCode != 0 || c.FfmpegTotalErrorLines > 0)
	return c.PrescanError != nil || len(c.Anomalies) > 0 || ffmpegReportedErrors
}

func (t TorrentCheck) ContentMatching() bool {
	noBadPieces := t.BadPieceCount == 0 && t.ZeroPieceCount == 0 && t.UnverifiablePieces == 0
	return noBadPieces && t.CountFiles(healthyFileStatus) == len(t.Files)
}

func (t TorrentCheck) CountFiles(status string) int {
	count := 0
	for _, file := range t.Files {
		if file.Status == status {
			count++
		}
	}
	return count
}
