package config

// TLSConfig holds the TLS settings of a client connection (IEEE 1815-2012 Annex B,
// IEC 62351-3). They apply to tls:// URLs, or to --ip when --tls is set.
type TLSConfig struct {
	TLS        bool   `env:"DNP3CTL_TLS" desc:"Use TLS with --ip (implied by a tls:// URL)" flag:"tls"`
	CACert     string `env:"DNP3CTL_TLS_CA" desc:"PEM file with the CA certificates to verify the outstation with (default: system roots)" flag:"tls-ca"`
	Cert       string `env:"DNP3CTL_TLS_CERT" desc:"PEM file with the own certificate (client authentication)" flag:"tls-cert"`
	Key        string `env:"DNP3CTL_TLS_KEY" desc:"PEM file with the private key of --tls-cert" flag:"tls-key"`
	ServerName string `env:"DNP3CTL_TLS_SERVER_NAME" desc:"Name to verify the outstation certificate against (default: the host)" flag:"tls-server-name"`
	Insecure   bool   `env:"DNP3CTL_TLS_INSECURE" desc:"Do not verify the outstation certificate (testing only)" flag:"tls-insecure"`
}

// SerialConfig holds the line settings of a serial port (serial:// URLs, or --serial on a
// server).
type SerialConfig struct {
	Baud     uint32 `env:"DNP3CTL_BAUD" desc:"Serial baud rate (0 = 9600)" flag:"baud"`
	DataBits uint8  `env:"DNP3CTL_DATA_BITS" desc:"Serial data bits, 5-8 (0 = 8)" flag:"data-bits"`
	Parity   string `env:"DNP3CTL_PARITY" desc:"Serial parity: none, even or odd (default none)" flag:"parity"`
	StopBits uint8  `env:"DNP3CTL_STOP_BITS" desc:"Serial stop bits, 1 or 2 (0 = 1)" flag:"stop-bits"`
}

// LinkConfig holds the data link layer settings. Zero selects the library default.
type LinkConfig struct {
	LinkConfirmed bool   `env:"DNP3CTL_LINK_CONFIRMED" desc:"Send user data as CONFIRMED_USER_DATA (default unconfirmed, as IEEE 1815-2012 recommends on TCP)" flag:"link-confirmed"`
	LinkTimeout   uint32 `env:"DNP3CTL_LINK_TIMEOUT" desc:"Wait in milliseconds for a link layer answer (0 = 2000)" flag:"link-timeout"`
	LinkRetries   uint8  `env:"DNP3CTL_LINK_RETRIES" desc:"Repeats of an unanswered primary link frame" flag:"link-retries"`
	KeepAlive     uint32 `env:"DNP3CTL_KEEP_ALIVE" desc:"Idle seconds after which REQUEST_LINK_STATUS is sent (0 = 60)" flag:"keep-alive"`
}

// AuthConfig holds the Secure Authentication v5 settings (pre-shared session key). The key
// is never given on the command line: it is read from --sa-key-file, or from the
// environment variable DNP3CTL_SA_KEY, as hex.
type AuthConfig struct {
	SAKeyFile string `env:"DNP3CTL_SA_KEY_FILE" desc:"File holding the pre-shared session key of Secure Authentication v5, as hex (or set DNP3CTL_SA_KEY); enables secure authentication" flag:"sa-key-file"`
	SAUser    uint16 `env:"DNP3CTL_SA_USER" desc:"Secure authentication user number" flag:"sa-user"`
	SAMAC     string `env:"DNP3CTL_SA_MAC" desc:"Secure authentication MAC algorithm: hmac-sha256-16 (default), hmac-sha256-8, hmac-sha1-10 or hmac-sha1-8" flag:"sa-mac"`
	SAKey     string `env:"DNP3CTL_SA_KEY"` // hex; environment only, never a flag
}

// AddressConfig holds the link addresses of a client.
type AddressConfig struct {
	Master     uint16 `env:"DNP3CTL_MASTER" desc:"Link address of this master" flag:"master"`
	Outstation uint16 `env:"DNP3CTL_OUTSTATION" desc:"Link address of the outstation" flag:"outstation"`
}

// ConnConfig is what every client command needs to reach an outstation.
type ConnConfig struct {
	TLSConfig
	SerialConfig
	LinkConfig
	AuthConfig
	AddressConfig
	URL             string `env:"DNP3CTL_URL" desc:"Outstation URL: tcp://host[:port], tls://host[:port], udp://host[:port] or serial:///dev/ttyUSB0 (ports default to 20000, and 19999 for tls); mutually exclusive with --ip/--port" flag:"url"`
	IP              string `env:"DNP3CTL_IP" desc:"Outstation IP address or host name, over TCP (used when --url is not set)" flag:"ip"`
	Port            uint16 `env:"DNP3CTL_PORT" desc:"Outstation TCP port (used when --url is not set)" flag:"port"`
	Timeout         uint32 `env:"DNP3CTL_TIMEOUT" desc:"Timeout in milliseconds for connecting and for each request, retries included (0 = 10000)" flag:"timeout"`
	ResponseTimeout uint32 `env:"DNP3CTL_RESPONSE_TIMEOUT" desc:"Wait in milliseconds for each fragment of a response (0 = 5000)" flag:"response-timeout"`
	Retries         uint8  `env:"DNP3CTL_RETRIES" desc:"Retries of a request that timed out or lost its connection; only for requests that are safe to repeat (reads, time, unsolicited control, assign class, file reads), never for controls, freezes, restarts or writes" flag:"retries"`
	Debug           bool   // set from global --debug (root persistent flag)
}

// PollConfig drives `client poll`.
type PollConfig struct {
	ConnConfig
	Mode         string `env:"DNP3CTL_POLL_MODE" desc:"What to read: integrity (events of classes 1-3, then the static data of class 0), event (events of --class) or class (exactly the classes of --class, 0 included)" flag:"mode"`
	Classes      string `env:"DNP3CTL_CLASS" desc:"Classes for --mode event (1-3) or class (0-3), comma-separated (default 1,2,3)" flag:"class"`
	Objects      string `env:"DNP3CTL_OBJECT" desc:"Only show these objects: groups or group/variations, comma-separated (e.g. g30,g1v2,32)" flag:"object"`
	Indexes      string `env:"DNP3CTL_INDEX" desc:"Only show these point indexes: list and ranges (e.g. 0,5-9)" flag:"index"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// ReadConfig drives `client read`.
type ReadConfig struct {
	ConnConfig
	Objects      string `env:"DNP3CTL_OBJECT" desc:"Objects to read, comma-separated: g30v5 (or 30.5), or g30 / g30v0 for any variation" flag:"object"`
	Range        string `env:"DNP3CTL_RANGE" desc:"Point indexes to read: a range (0-9), a list (1,4,7) or empty for all points" flag:"range"`
	Count        uint32 `env:"DNP3CTL_COUNT" desc:"Read at most this many objects (events); cannot be combined with --range" flag:"count"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// OperateConfig drives `client operate`.
type OperateConfig struct {
	ConnConfig
	Type         string `env:"DNP3CTL_CONTROL_TYPE" desc:"Control: crob (control relay output block, g12v1) or analog (analog output block, g41)" flag:"type"`
	Index        uint32 `env:"DNP3CTL_OPERATE_INDEX" desc:"Point index of the output" flag:"index"`
	Op           string `env:"DNP3CTL_OP" desc:"Operation of a crob: latch-on, latch-off, pulse-on, pulse-off, close (pulse on, trip-close code close), trip (pulse on, trip-close code trip) or nul" flag:"op"`
	Count        uint8  `env:"DNP3CTL_OP_COUNT" desc:"Number of times a crob is executed" flag:"count"`
	OnTime       uint32 `env:"DNP3CTL_ON_TIME" desc:"On time of a pulse in milliseconds" flag:"on-time"`
	OffTime      uint32 `env:"DNP3CTL_OFF_TIME" desc:"Off time of a pulse in milliseconds" flag:"off-time"`
	Value        string `env:"DNP3CTL_VALUE" desc:"Set-point value of an analog control" flag:"value"`
	Variation    uint8  `env:"DNP3CTL_VARIATION" desc:"Variation of an analog control: 1 (32-bit), 2 (16-bit), 3 (single-precision float), 4 (double-precision float); 0 = 1 for whole numbers that fit, 3 otherwise" flag:"variation"`
	Mode         string `env:"DNP3CTL_OPERATE_MODE" desc:"Procedure: sbo (select, then operate), direct (direct operate) or direct-no-ack (direct operate, no response)" flag:"mode"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// MonitorConfig drives `client monitor`.
type MonitorConfig struct {
	ConnConfig
	Integrity    bool   `env:"DNP3CTL_INTEGRITY" desc:"Run an integrity poll after connecting (and after every reconnect), so the stream starts with all current values" flag:"integrity"`
	Unsolicited  string `env:"DNP3CTL_UNSOLICITED" desc:"Classes to enable unsolicited responses for after connecting, comma-separated, or none" flag:"unsolicited"`
	PollInterval uint32 `env:"DNP3CTL_POLL_INTERVAL" desc:"Also poll the events of classes 1-3 every this many seconds (0 = do not poll)" flag:"poll-interval"`
	Objects      string `env:"DNP3CTL_OBJECT" desc:"Only show these objects: groups or group/variations, comma-separated" flag:"object"`
	Indexes      string `env:"DNP3CTL_INDEX" desc:"Only show these point indexes: list and ranges" flag:"index"`
	Duration     uint32 `env:"DNP3CTL_DURATION" desc:"Stop after this many seconds (0 = until interrupted)" flag:"duration"`
	Count        uint32 `env:"DNP3CTL_COUNT" desc:"Stop after this many values (0 = until interrupted)" flag:"count"`
	NoReconnect  bool   `env:"DNP3CTL_NO_RECONNECT" desc:"Exit when the connection is lost instead of reconnecting" flag:"no-reconnect"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// PingConfig drives `client ping`.
type PingConfig struct {
	ConnConfig
	Count        uint32 `env:"DNP3CTL_COUNT" desc:"Number of delay measurements before exiting (0 = until interrupted)" flag:"count"`
	Interval     uint32 `env:"DNP3CTL_INTERVAL" desc:"Interval between measurements in milliseconds" flag:"interval"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// TimeSyncConfig drives `client time-sync`.
type TimeSyncConfig struct {
	ConnConfig
	Method       string `env:"DNP3CTL_TIME_METHOD" desc:"Procedure: lan (RECORD_CURRENT_TIME, then the time of that request), non-lan (DELAY_MEASURE, then the time corrected by the delay) or write (the time of --time, without correction)" flag:"method"`
	Time         string `env:"DNP3CTL_TIME" desc:"Time to write with --method write: now, or an RFC 3339 time such as 2026-01-31T12:00:00Z" flag:"time"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// RestartConfig drives `client restart`.
type RestartConfig struct {
	ConnConfig
	Type         string `env:"DNP3CTL_RESTART_TYPE" desc:"Restart: cold (full restart of the outstation) or warm (restart of the DNP3 application)" flag:"type"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// UnsolicitedConfig drives `client unsolicited enable` and `disable`.
type UnsolicitedConfig struct {
	ConnConfig
	Classes      string `env:"DNP3CTL_CLASS" desc:"Event classes, comma-separated, 1-3 (default all three)" flag:"class"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// AssignClassConfig drives `client assign-class`.
type AssignClassConfig struct {
	ConnConfig
	Class        uint8  `env:"DNP3CTL_ASSIGN_CLASS" desc:"Class to assign, 0-3 (0 = no events)" flag:"class"`
	Objects      string `env:"DNP3CTL_OBJECT" desc:"Static objects whose points are assigned, comma-separated (e.g. g30, g1)" flag:"object"`
	Range        string `env:"DNP3CTL_RANGE" desc:"Point indexes: a range (0-9), a list (1,4,7) or empty for all points" flag:"range"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// FreezeConfig drives `client freeze`.
type FreezeConfig struct {
	ConnConfig
	Objects      string `env:"DNP3CTL_OBJECT" desc:"Objects to freeze, comma-separated: g20 (counters) or g30 (analog inputs)" flag:"object"`
	Range        string `env:"DNP3CTL_RANGE" desc:"Point indexes: a range (0-9), a list (1,4,7) or empty for all points" flag:"range"`
	Clear        bool   `env:"DNP3CTL_FREEZE_CLEAR" desc:"Reset the counters to zero after the freeze (FREEZE_CLEAR)" flag:"clear"`
	NoAck        bool   `env:"DNP3CTL_NO_ACK" desc:"Send the no-acknowledgement form (IMMED_FREEZE_NR, FREEZE_CLEAR_NR): no response" flag:"no-ack"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// AttributesConfig drives `client attributes`.
type AttributesConfig struct {
	ConnConfig
	Variation    string `env:"DNP3CTL_ATTRIBUTE" desc:"Attribute to read: all, list (the list of attribute variations), a variation number 1-254 or a name such as manufacturer" flag:"variation"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// FileAuthConfig holds the credentials of AUTHENTICATE_FILE. The password is never given on
// the command line: it is read from --file-password-file or DNP3CTL_FILE_PASSWORD.
type FileAuthConfig struct {
	FileUser         string `env:"DNP3CTL_FILE_USER" desc:"User name to authenticate file operations with (AUTHENTICATE_FILE); the password comes from --file-password-file or DNP3CTL_FILE_PASSWORD" flag:"file-user"`
	FilePasswordFile string `env:"DNP3CTL_FILE_PASSWORD_FILE" desc:"File holding the password of --file-user" flag:"file-password-file"`
	FilePassword     string `env:"DNP3CTL_FILE_PASSWORD"` // environment only, never a flag
	BlockSize        uint16 `env:"DNP3CTL_BLOCK_SIZE" desc:"Largest block to ask for or send, in octets (0 = the outstation chooses)" flag:"block-size"`
}

// FileListConfig drives `client file ls`.
type FileListConfig struct {
	ConnConfig
	FileAuthConfig
	Path         string `env:"DNP3CTL_FILE_PATH" desc:"Directory to list" flag:"path"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// FileInfoConfig drives `client file info`.
type FileInfoConfig struct {
	ConnConfig
	Path         string `env:"DNP3CTL_FILE_PATH" desc:"File or directory on the outstation" flag:"path"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// FileGetConfig drives `client file get`.
type FileGetConfig struct {
	ConnConfig
	FileAuthConfig
	Path         string `env:"DNP3CTL_FILE_PATH" desc:"File on the outstation" flag:"path"`
	Output       string `env:"DNP3CTL_OUTPUT" desc:"File to write the content to (- = stdout; default: the base name of --path)" flag:"output"`
	Force        bool   `env:"DNP3CTL_FORCE" desc:"Overwrite an existing --output file" flag:"force"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// FilePutConfig drives `client file put`.
type FilePutConfig struct {
	ConnConfig
	FileAuthConfig
	Path         string `env:"DNP3CTL_FILE_PATH" desc:"File to create on the outstation" flag:"path"`
	Input        string `env:"DNP3CTL_INPUT" desc:"Local file whose content is sent" flag:"input"`
	Permissions  string `env:"DNP3CTL_PERMISSIONS" desc:"Permissions of the new file: octal (644) or rwx form (rw-r--r--)" flag:"permissions"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// FileDeleteConfig drives `client file rm`.
type FileDeleteConfig struct {
	ConnConfig
	FileAuthConfig
	Path         string `env:"DNP3CTL_FILE_PATH" desc:"File to delete on the outstation" flag:"path"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// ObjectsConfig drives `client objects`.
type ObjectsConfig struct {
	Group        uint8  `env:"DNP3CTL_GROUP" desc:"Only list the variations of this group (0 = all groups)" flag:"group"`
	Events       bool   `env:"DNP3CTL_EVENTS" desc:"Only list event objects" flag:"events"`
	OutputFormat string `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// DiscoverConfig drives `discover`.
type DiscoverConfig struct {
	Subnets      []string `env:"DNP3CTL_SUBNETS" desc:"Subnets, ranges or addresses to probe: CIDR (192.168.1.0/24), range (192.168.1.10-20) or a single address; comma-separated" flag:"subnets"`
	Ports        []string `env:"DNP3CTL_PORTS" desc:"TCP ports to probe" flag:"ports"`
	Outstations  []string `env:"DNP3CTL_OUTSTATIONS" desc:"Link addresses to send the probe to, comma-separated; 65532 is the self address, which outstations that support it answer whatever their own address" flag:"outstations"`
	Master       uint16   `env:"DNP3CTL_MASTER" desc:"Link address the probe is sent from" flag:"master"`
	Timeout      uint32   `env:"DNP3CTL_TIMEOUT" desc:"Timeout in milliseconds per probe (0 = 1000)" flag:"timeout"`
	Parallel     uint16   `env:"DNP3CTL_PARALLEL" desc:"Number of concurrent probes (1-256)" flag:"parallel"`
	ForceLarge   bool     `env:"DNP3CTL_DISCOVER_FORCE_LARGE" desc:"Allow a scan of more hosts than the built-in safety cap" flag:"force-large-scan"`
	OutputFormat string   `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
	Debug        bool     // set from global --debug (root persistent flag)
}

// ServerConfig drives `server start`.
type ServerConfig struct {
	LinkConfig
	SerialConfig
	AuthConfig
	Config       string   `env:"DNP3CTL_CONFIG" desc:"Outstation model file (YAML or JSON); without it a built-in demo outstation is served" flag:"config"`
	Listen       string   `env:"DNP3CTL_LISTEN" desc:"Address to listen on (TCP, TLS and UDP)" flag:"listen"`
	Port         uint16   `env:"DNP3CTL_PORT" desc:"Port to listen on (0 = 20000, or 19999 with TLS)" flag:"port"`
	UDP          bool     `env:"DNP3CTL_UDP" desc:"Serve DNP3 over UDP instead of TCP (one master at a time)" flag:"udp"`
	Serial       string   `env:"DNP3CTL_SERIAL" desc:"Serve on this serial port (e.g. /dev/ttyUSB0 or COM3) instead of the network; line settings with --baud, --data-bits, --parity, --stop-bits" flag:"serial"`
	Cert         string   `env:"DNP3CTL_TLS_CERT" desc:"PEM file with the server certificate; with --tls-key it makes the server a TLS server" flag:"tls-cert"`
	Key          string   `env:"DNP3CTL_TLS_KEY" desc:"PEM file with the private key of --tls-cert" flag:"tls-key"`
	ClientCA     string   `env:"DNP3CTL_TLS_CLIENT_CA" desc:"PEM file with the CA certificates masters must present a certificate of (mutual TLS)" flag:"tls-client-ca"`
	Allow        []string `env:"DNP3CTL_ALLOW" desc:"Only accept TCP and TLS clients from these addresses or CIDR prefixes (default: any)" flag:"allow"`
	MaxSessions  uint16   `env:"DNP3CTL_MAX_SESSIONS" desc:"Maximum number of simultaneous connections (0 = 16)" flag:"max-sessions"`
	Duration     uint32   `env:"DNP3CTL_DURATION" desc:"Stop after this many seconds (0 = until interrupted)" flag:"duration"`
	OutputFormat string   `env:"DNP3CTL_OUTPUT_FORMAT" desc:"Format of the event log on stdout: text, json, or table" flag:"format"`
	Debug        bool     // set from global --debug (root persistent flag)
}

// GenerateConfigConfig drives `server generate-config`.
type GenerateConfigConfig struct {
	Output string `env:"DNP3CTL_OUTPUT" desc:"File to write the model to (- = stdout)" flag:"output"`
	Format string `env:"DNP3CTL_CONFIG_FORMAT" desc:"Format of the model: yaml or json" flag:"format"`
	Force  bool   `env:"DNP3CTL_FORCE" desc:"Overwrite an existing file" flag:"force"`
}

// WantsTLS reports whether a TLS flag is set.
func (c *ConnConfig) WantsTLS() bool {
	t := c.TLSConfig
	return t.TLS || t.CACert != "" || t.Cert != "" || t.Key != "" || t.ServerName != "" || t.Insecure
}
