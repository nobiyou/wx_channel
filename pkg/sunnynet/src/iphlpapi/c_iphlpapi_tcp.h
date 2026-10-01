#include <windows.h>
#include <iphlpapi.h>
#include <stdio.h>

// MIB_TCPROW2、MIB_TCPTABLE2 和 GetTcpTable2 由 Windows SDK 的
// iphlpapi.h 提供。旧版 SunnyNet 在这里重复声明它们，会与现代
// MinGW/Windows SDK 头文件冲突，因此只保留本地动态加载所需的类型。

// 定义 GETEXTENDEDTABLE 函数指针类型
typedef DWORD(WINAPI* GETEXTENDEDTABLE)(PVOID, PDWORD, BOOL, ULONG, TCP_TABLE_CLASS, ULONG);

// 定义 SETTCPENTRY 函数指针类型
typedef DWORD(WINAPI* SETTCPENTRY)(PMIB_TCPROW);

// 定义 GetTcpTable2 函数指针类型（避免与 SDK 函数声明重名）
typedef DWORD(WINAPI* GETTCPTABLE2)(PMIB_TCPTABLE2 TcpTable, PULONG SizePointer, BOOL Order);

// 关闭 TCP 连接初始化
void closeTcpConnectionInit();

// 根据 PID 关闭 TCP 连接
void closeTcpConnectionByPid(DWORD pid, DWORD ulAf);

// 获取指定 TCP 地址和端口的 PID
int getTcpInfoPID(char* Addr, int SunnyProt);