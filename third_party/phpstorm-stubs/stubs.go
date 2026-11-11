// Package phpstormstubs embeds the PHP stubs from JetBrains phpstorm-stubs
// (https://github.com/JetBrains/phpstorm-stubs, Apache-2.0; see LICENSE):
// declarations of the functions, classes and constants built into PHP, so
// they can be indexed like library code.
//
// Only the extensions bundled with PHP itself are included. To update, copy
// the same directories and LICENSE from a newer tag and change Version.
package phpstormstubs

import "embed"

// Version is the phpstorm-stubs tag the files were taken from.
const Version = "v2026.2"

// Files holds the stubs, one directory per PHP extension, and LICENSE.
//
//go:embed LICENSE
//go:embed all:Core
//go:embed all:FFI
//go:embed all:PDO
//go:embed all:Phar
//go:embed all:Reflection
//go:embed all:SPL
//go:embed all:SimpleXML
//go:embed "all:Zend OPcache"
//go:embed all:apache
//go:embed all:bcmath
//go:embed all:bz2
//go:embed all:calendar
//go:embed all:com_dotnet
//go:embed all:ctype
//go:embed all:curl
//go:embed all:date
//go:embed all:dba
//go:embed all:dom
//go:embed all:enchant
//go:embed all:exif
//go:embed all:fileinfo
//go:embed all:filter
//go:embed all:fpm
//go:embed all:ftp
//go:embed all:gd
//go:embed all:gettext
//go:embed all:gmp
//go:embed all:hash
//go:embed all:iconv
//go:embed all:imap
//go:embed all:intl
//go:embed all:json
//go:embed all:ldap
//go:embed all:libxml
//go:embed all:mbstring
//go:embed all:mysqli
//go:embed all:oci8
//go:embed all:odbc
//go:embed all:openssl
//go:embed all:pcntl
//go:embed all:pcre
//go:embed all:pgsql
//go:embed all:posix
//go:embed all:pspell
//go:embed all:random
//go:embed all:readline
//go:embed all:session
//go:embed all:shmop
//go:embed all:snmp
//go:embed all:soap
//go:embed all:sockets
//go:embed all:sodium
//go:embed all:sqlite3
//go:embed all:standard
//go:embed all:superglobals
//go:embed all:sysvmsg
//go:embed all:sysvsem
//go:embed all:sysvshm
//go:embed all:tidy
//go:embed all:tokenizer
//go:embed all:uri
//go:embed all:xml
//go:embed all:xmlreader
//go:embed all:xmlwriter
//go:embed all:xsl
//go:embed all:zip
//go:embed all:zlib
var Files embed.FS
