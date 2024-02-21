package internal

import "strconv"

type Counter struct {
	TotalCount   int
	SucceedCount int
	FailedCount  int
}

func (c Counter) String() string {
	return "total " + strconv.Itoa(c.TotalCount) + ", succeed " + strconv.Itoa(c.SucceedCount) + ", failed " + strconv.Itoa(c.FailedCount)
}
