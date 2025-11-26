package model

type Request struct {
	OsImage      string
	Ram          int
	Cpu          int
	RequestId    string
	Namespace    string
	VMName       string
	Architecture string
	HostName     string
}

type DeclaredVM struct {
	RequestInfo Request
	ID          string
}
