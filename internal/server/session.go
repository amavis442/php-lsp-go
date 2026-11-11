package server

import "go.lsp.dev/protocol"

// Session is what initialize told the server about the editor's workspace
// and abilities.
type Session struct {
	// Roots are the file URIs of the workspace folders, or of the root
	// when the editor has no folders. Empty when a lone file is open.
	Roots []string
	// WorkDoneProgress means the editor shows server-created progress.
	WorkDoneProgress bool
	// WatchedFilesRegistration means the server may register file watchers.
	WatchedFilesRegistration bool
}

func sessionFrom(params *protocol.InitializeParams) Session {
	var s Session
	if folders, ok := params.WorkspaceFolders.Get(); ok && len(folders) > 0 {
		for _, f := range folders {
			s.Roots = append(s.Roots, string(f.URI))
		}
	} else if params.RootURI != nil && *params.RootURI != "" {
		s.Roots = []string{string(*params.RootURI)}
	}

	caps := params.Capabilities
	if caps.Window != nil && caps.Window.WorkDoneProgress != nil {
		s.WorkDoneProgress = *caps.Window.WorkDoneProgress
	}
	if caps.Workspace != nil && caps.Workspace.DidChangeWatchedFiles != nil &&
		caps.Workspace.DidChangeWatchedFiles.DynamicRegistration != nil {
		s.WatchedFilesRegistration = *caps.Workspace.DidChangeWatchedFiles.DynamicRegistration
	}
	return s
}
