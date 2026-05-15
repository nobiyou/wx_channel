#include "c_iphlpapi_tcp.h"

// 定义指向 GetTcpTable2 函数的指针变量
GetTcpTable2 pGetTcpTable2;

// 定义指向 GetExtendedTcpTable 函数的指针变量
GETEXTENDEDTABLE pGetExtendedTcpTable;

// 定义指向 SetTcpEntry 函数的指针变量
SETTCPENTRY pSetTcpEntry;

// 初始化关闭 TCP 连接函数
void closeTcpConnectionInit() {
	// 加载 iphlpapi.dll 动态库
	HMODULE hModule = LoadLibrary("iphlpapi.dll");
	if (hModule == NULL) {
		return;
		// 加载失败，退出函数
	}

	// 获取 GetExtendedTcpTable 函数地址
	pGetExtendedTcpTable = (GETEXTENDEDTABLE)GetProcAddress(hModule, "GetExtendedTcpTable");

	// 获取 SetTcpEntry 函数地址
	pSetTcpEntry = (SETTCPENTRY)GetProcAddress(hModule, "SetTcpEntry");

	// 获取 GetTcpTable2 函数地址
	pGetTcpTable2 = (GetTcpTable2) GetProcAddress(hModule, "GetTcpTable2");

	if (pGetExtendedTcpTable == NULL || pSetTcpEntry == NULL || pGetTcpTable2 == NULL) {
		// 获取失败，释放动态库并退出函数
		FreeLibrary(hModule);
		return;
	}
}

// 将指定的 TCP 连接关闭
void closeTcpConnectionByPid(DWORD pid, DWORD ulAf) {
	// 如果函数指针变量未初始化，退出函数
	if (pGetExtendedTcpTable == NULL || pSetTcpEntry == NULL) {
		return;
	}

	// 定义指向 TCP 连接表的指针变量
	MIB_TCPTABLE_OWNER_PID* tcpTable = NULL;

	// TCP 连接表的大小
	DWORD tcpTableSize = 0;

	// Windows API 函数调用结果
	DWORD result = 0;

	// 获取 TCP 连接列表
	result = pGetExtendedTcpTable(NULL, &tcpTableSize, TRUE, ulAf, TCP_TABLE_OWNER_PID_ALL, 0);
	if (result == ERROR_INSUFFICIENT_BUFFER) {
		tcpTable = (MIB_TCPTABLE_OWNER_PID*)malloc(tcpTableSize);
		// 分配内存空间
		result = pGetExtendedTcpTable(tcpTable, &tcpTableSize, TRUE, ulAf, TCP_TABLE_OWNER_PID_ALL, 0);
		// 获取 TCP 连接列表
		if (result == NO_ERROR) {
			// 遍历 TCP 连接列表，查找指定 PID 的连接
			for (DWORD i = 0; i < tcpTable->dwNumEntries; i++) {
				MIB_TCPROW_OWNER_PID* tcpRow = &tcpTable->table[i];
				if ((pid == -1 && tcpRow->dwState == MIB_TCP_STATE_ESTAB) || (tcpRow->dwOwningPid == pid && tcpRow->dwState == MIB_TCP_STATE_ESTAB)) {
					// 关闭指定的 TCP 连接
					MIB_TCPROW tcpRow2;
					tcpRow2.dwState = MIB_TCP_STATE_DELETE_TCB;
					tcpRow2.dwLocalAddr = tcpRow->dwLocalAddr;
					tcpRow2.dwLocalPort = tcpRow->dwLocalPort;
					tcpRow2.dwRemoteAddr = tcpRow->dwRemoteAddr;
					tcpRow2.dwRemotePort = tcpRow->dwRemotePort;
					pSetTcpEntry(&tcpRow2);
				}
			}
		}
		free(tcpTable);
		// 释放内存空间
	}
}

// 将网络字节序转换为主机字节序
int ntohs2(u_short v) {
	return (int)((u_short)(v >> 8) | (u_short)(v << 8));
}

// 获取指定 TCP 地址和端口的 PID
int getTcpInfoPID(char* Addr, int SunnyProt) {
	if (pGetTcpTable2 == NULL) {
		return -1;
	}
	ULONG bufferSize = 0;

	// Windows API 函数调用结果
	DWORD result = pGetTcpTable2(NULL, &bufferSize, TRUE);
	if (result != ERROR_INSUFFICIENT_BUFFER) {
		// 获取 TCP 连接列表失败，退出函数
		return -2;
	}

	// 分配内存空间
	PMIB_TCPTABLE2 tcpTable = (PMIB_TCPTABLE2)malloc(bufferSize);
	result = pGetTcpTable2(tcpTable, &bufferSize, TRUE);
	if (result != NO_ERROR){
		free(tcpTable);
		// 获取 TCP 连接列表失败，释放内存空间后退出函数
		return -3;
	}

	// 定义缓存字符串
	char buf[64];
	// 遍历 TCP 连接列表，查找指定地址和端口的连接的 PID
	for (DWORD i = 0; i < tcpTable->dwNumEntries; i++) {
		sprintf(buf, "%d.%d.%d.%d:%d",
			(tcpTable->table[i].dwLocalAddr >> 0) & 0xff,
			(tcpTable->table[i].dwLocalAddr >> 8) & 0xff,
			(tcpTable->table[i].dwLocalAddr >> 16) & 0xff,
			(tcpTable->table[i].dwLocalAddr >> 24) & 0xff,
			ntohs2((u_short)tcpTable->table[i].dwLocalPort));
		int cmpResult = strcmp(buf, Addr);
		if (cmpResult==0) {
			// 找到指定连接，返回 PID
			int r =(int)(tcpTable->table[i].dwOwningPid);
			free(tcpTable);
			return r;
		}
	}

	free(tcpTable);
	// 没有找到指定连接，释放内存空间后返回错误代码
	return -4;
}