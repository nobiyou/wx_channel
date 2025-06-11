/* Driver.h */

typedef  struct _NF_UDP_OPTIONS
{
	unsigned long	flags;		// Datagram flags
	long			optionsLength;	// Length of options buffer
	unsigned char	options[1]; // Options of variable size
} NF_UDP_OPTIONS, *PNF_UDP_OPTIONS;


typedef  struct _NF_UDP_CONN_REQUEST
{
	unsigned long	filteringFlag;
	unsigned long	processId;
	unsigned short	ip_family;
	unsigned char localAddress[16];
	unsigned char remoteAddress[16];
} NF_UDP_CONN_REQUEST, *PNF_UDP_CONN_REQUEST;

typedef  struct _NF_UDP_CONN_INFO
{
	unsigned long	processId;
	unsigned short	ip_family;
	unsigned char localAddress[16];
} NF_UDP_CONN_INFO, *PNF_UDP_CONN_INFO;
typedef  struct _NF_TCP_CONN_INFO
{
	unsigned long	filteringFlag;
	unsigned long	processId;
	unsigned char	direction;
	unsigned short	ip_family;
	unsigned char localAddress[16];
	unsigned char remoteAddress[16];
} NF_TCP_CONN_INFO, *PNF_TCP_CONN_INFO;


typedef unsigned  long long ENDPOINT_ID;

typedef struct _NF_EventHandler
{
	void (__cdecl *threadStart)();
	void (__cdecl *threadEnd)();
	void (__cdecl *tcpConnectRequest)( ENDPOINT_ID id, PNF_TCP_CONN_INFO pConnInfo );
	void (__cdecl *tcpConnected)( ENDPOINT_ID id, PNF_TCP_CONN_INFO pConnInfo );
	void (__cdecl *tcpClosed)( ENDPOINT_ID id, PNF_TCP_CONN_INFO pConnInfo );
	void (__cdecl *tcpReceive)( ENDPOINT_ID id, const char * buf, int len );
	void (__cdecl *tcpSend)( ENDPOINT_ID id, const char * buf, int len );
	void (__cdecl *tcpCanReceive)( ENDPOINT_ID id );
	void (__cdecl *tcpCanSend)( ENDPOINT_ID id );
	void (__cdecl *udpCreated)( ENDPOINT_ID id, PNF_UDP_CONN_INFO pConnInfo );
	void (__cdecl *udpConnectRequest)( ENDPOINT_ID id, PNF_UDP_CONN_REQUEST pConnReq );
	void (__cdecl *udpClosed)( ENDPOINT_ID id, PNF_UDP_CONN_INFO pConnInfo );
	void (__cdecl *udpReceive)( ENDPOINT_ID id, const unsigned char * remoteAddress, const char * buf, int len, PNF_UDP_OPTIONS options );
	void (__cdecl *udpSend)( ENDPOINT_ID id, const unsigned char * remoteAddress, const char * buf, int len, PNF_UDP_OPTIONS options );
	void (__cdecl *udpCanReceive)( ENDPOINT_ID id );
	void (__cdecl *udpCanSend)( ENDPOINT_ID id );
} NF_EventHandler, *PNF_EventHandler;
typedef enum _NF_STATUS
{
	NF_STATUS_SUCCESS		= 0,
	NF_STATUS_FAIL			= -1,
	NF_STATUS_INVALID_ENDPOINT_ID	= -2,
	NF_STATUS_NOT_INITIALIZED	= -3,
	NF_STATUS_IO_ERROR		= -4,
	NF_STATUS_REBOOT_REQUIRED	= -5
} NF_STATUS;


int NfDriverInit( char *, void * );

NF_STATUS A1(void * addr ,ENDPOINT_ID id, const unsigned char *remoteAddress, const char *buf, int len, PNF_UDP_OPTIONS options);

