
#include "Driver.h"


typedef int (*Fun0)( char*, void* );
typedef NF_STATUS (*UdpPostSend)(ENDPOINT_ID id, const unsigned char *remoteAddress, const char *buf, int len, PNF_UDP_OPTIONS options);

/* C调Golang函数 */
extern void go_threadStart();


extern void go_threadEnd();


extern void go_tcpConnectRequest( ENDPOINT_ID id, PNF_TCP_CONN_INFO pConnInfo );


extern void go_tcpConnected( ENDPOINT_ID id, PNF_TCP_CONN_INFO pConnInfo );


extern void go_tcpClosed( ENDPOINT_ID id, PNF_TCP_CONN_INFO pConnInfo );


extern void go_tcpReceive( ENDPOINT_ID id, const char * buf, int len );


extern void go_tcpSend( ENDPOINT_ID id, const char * buf, int len );


extern void go_tcpCanReceive( ENDPOINT_ID id );


extern void go_tcpCanSend( ENDPOINT_ID id );


extern void go_udpCreated( ENDPOINT_ID id, PNF_UDP_CONN_INFO pConnInfo );


extern void go_udpConnectRequest( ENDPOINT_ID id, PNF_UDP_CONN_REQUEST pConnReq );


extern void go_udpClosed( ENDPOINT_ID id, PNF_UDP_CONN_INFO pConnInfo );


extern void go_udpReceive( ENDPOINT_ID id, const unsigned char * remoteAddress, const char * buf, int len, PNF_UDP_OPTIONS options );


extern void go_udpSend( ENDPOINT_ID id, const unsigned char * remoteAddress, const char * buf, int len, PNF_UDP_OPTIONS options );


extern void go_udpCanReceive( ENDPOINT_ID id );


extern void go_udpCanSend( ENDPOINT_ID id );


void threadStart()
{
	go_threadStart();
}


void threadEnd()
{
	go_threadEnd();
}


void tcpConnectRequest( ENDPOINT_ID id, PNF_TCP_CONN_INFO pConnInfo )
{
	go_tcpConnectRequest( id, pConnInfo );
}


void tcpConnected( ENDPOINT_ID id, PNF_TCP_CONN_INFO pConnInfo )
{
	go_tcpConnected( id, pConnInfo );
}


void tcpClosed( ENDPOINT_ID id, PNF_TCP_CONN_INFO pConnInfo )
{
	go_tcpClosed( id, pConnInfo );
}


void tcpReceive( ENDPOINT_ID id, const char * buf, int len )
{
	go_tcpReceive( id, buf, len );
}


void tcpSend( ENDPOINT_ID id, const char * buf, int len )
{
	go_tcpSend( id, buf, len );
}


void tcpCanReceive( ENDPOINT_ID id )
{
	go_tcpCanReceive( id );
}


void tcpCanSend( ENDPOINT_ID id )
{
	go_tcpCanSend( id );
}


void udpCreated( ENDPOINT_ID id, PNF_UDP_CONN_INFO pConnInfo )
{
	go_udpCreated( id, pConnInfo );
}


void udpConnectRequest( ENDPOINT_ID id, PNF_UDP_CONN_REQUEST pConnReq )
{
	go_udpConnectRequest( id, pConnReq );
}


void udpClosed( ENDPOINT_ID id, PNF_UDP_CONN_INFO pConnInfo )
{
	go_udpClosed( id, pConnInfo );
}


void udpReceive( ENDPOINT_ID id, const unsigned char * remoteAddress, const char * buf, int len, PNF_UDP_OPTIONS options )
{
	go_udpReceive( id, remoteAddress, buf, len, options );
}


void udpSend( ENDPOINT_ID id, const unsigned char * remoteAddress, const char * buf, int len, PNF_UDP_OPTIONS options )
{
	go_udpSend( id, remoteAddress, buf, len, options );
}


void udpCanReceive( ENDPOINT_ID id )
{
	go_udpCanReceive( id );
}


void udpCanSend( ENDPOINT_ID id )
{
	go_udpCanSend( id );
}


NF_EventHandler eh = {
	threadStart,
	threadEnd,
	tcpConnectRequest,
	tcpConnected,
	tcpClosed,
	tcpReceive,
	tcpSend,
	tcpCanReceive,
	tcpCanSend,
	udpCreated,
	udpConnectRequest,
	udpClosed,
	udpReceive,
	udpSend,
	udpCanReceive,
	udpCanSend
};


int NfDriverInit( char * driverName, void * addr )
{
    int r=( ( (Fun0) addr)( driverName, &eh ) );
	return r;
}


NF_STATUS A1(void * addr ,ENDPOINT_ID id, const unsigned char *remoteAddress, const char *buf, int len, PNF_UDP_OPTIONS options)
{
    int r=( ( (UdpPostSend) addr)( id, remoteAddress,buf,len,options ) );
	return r;
}

