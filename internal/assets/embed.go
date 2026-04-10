package assets

import (
	_ "embed"
)

//go:embed certs/SunnyRoot.cer
var CertData []byte

// MitmCACert and MitmCAKey are populated at runtime by certgen.EnsureCA()
// instead of being embedded, so that private keys are never committed to the repo.
var MitmCACert []byte
var MitmCAKey []byte

//go:embed lib/FileSaver.min.js
var FileSaverJS []byte

//go:embed lib/jszip.min.js
var ZipJS []byte

//go:embed inject/core.js
var CoreJS []byte

//go:embed inject/decrypt.js
var DecryptJS []byte

//go:embed inject/download.js
var DownloadJS []byte

//go:embed inject/home.js
var HomeJS []byte

//go:embed inject/feed.js
var FeedJS []byte

//go:embed inject/profile.js
var ProfileJS []byte

//go:embed inject/search.js
var SearchJS []byte

//go:embed inject/batch_download.js
var BatchDownloadJS []byte

//go:embed inject/lib/mitt.umd.js
var MittJS []byte

//go:embed inject/eventbus.js
var EventbusJS []byte

//go:embed inject/utils.js
var UtilsJS []byte

//go:embed inject/api_client.js
var APIClientJS []byte

//go:embed inject/keep_alive.js
var KeepAliveJS []byte

//go:embed proxy.pac
var ProxyPAC []byte
