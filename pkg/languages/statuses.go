package languages

// Status represents a Judge0 execution status.
type Status struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
}

// Judge0 Status Constants
const (
	StatusInQueue             = 1
	StatusProcessing          = 2
	StatusAccepted            = 3
	StatusWrongAnswer         = 4
	StatusTimeLimitExceeded   = 5
	StatusCompilationError    = 6
	StatusRuntimeErrorSIGSEGV = 7
	StatusRuntimeErrorSIGXFSZ = 8
	StatusRuntimeErrorSIGFPE  = 9
	StatusRuntimeErrorSIGABRT = 10
	StatusRuntimeErrorNZEC    = 11
	StatusRuntimeErrorOther   = 12
	StatusInternalError       = 13
	StatusExecFormatError     = 14
)

var statuses = map[int]Status{
	StatusInQueue:             {ID: StatusInQueue, Description: "In Queue"},
	StatusProcessing:          {ID: StatusProcessing, Description: "Processing"},
	StatusAccepted:            {ID: StatusAccepted, Description: "Accepted"},
	StatusWrongAnswer:         {ID: StatusWrongAnswer, Description: "Wrong Answer"},
	StatusTimeLimitExceeded:   {ID: StatusTimeLimitExceeded, Description: "Time Limit Exceeded"},
	StatusCompilationError:    {ID: StatusCompilationError, Description: "Compilation Error"},
	StatusRuntimeErrorSIGSEGV: {ID: StatusRuntimeErrorSIGSEGV, Description: "Runtime Error (SIGSEGV)"},
	StatusRuntimeErrorSIGXFSZ: {ID: StatusRuntimeErrorSIGXFSZ, Description: "Runtime Error (SIGXFSZ)"},
	StatusRuntimeErrorSIGFPE:  {ID: StatusRuntimeErrorSIGFPE, Description: "Runtime Error (SIGFPE)"},
	StatusRuntimeErrorSIGABRT: {ID: StatusRuntimeErrorSIGABRT, Description: "Runtime Error (SIGABRT)"},
	StatusRuntimeErrorNZEC:    {ID: StatusRuntimeErrorNZEC, Description: "Runtime Error (NZEC)"},
	StatusRuntimeErrorOther:   {ID: StatusRuntimeErrorOther, Description: "Runtime Error (Other)"},
	StatusInternalError:       {ID: StatusInternalError, Description: "Internal Error"},
	StatusExecFormatError:     {ID: StatusExecFormatError, Description: "Exec Format Error"},
}

// GetStatusByID returns the status corresponding to the ID or Internal Error if not found.
func GetStatusByID(id int) Status {
	if s, exists := statuses[id]; exists {
		return s
	}
	return Status{ID: StatusInternalError, Description: "Internal Error"}
}

// GetAllStatuses returns an ordered slice of all known statuses.
func GetAllStatuses() []Status {
	list := make([]Status, 0, len(statuses))
	for i := 1; i <= len(statuses); i++ {
		if s, ok := statuses[i]; ok {
			list = append(list, s)
		}
	}
	return list
}
