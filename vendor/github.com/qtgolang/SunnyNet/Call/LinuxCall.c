#include "LinuxCall.h"
#include <stdio.h>

typedef int (*Fun0)();
typedef int (*Fun1)(void *);
typedef int (*Fun2)(void *,void *);
typedef int (*Fun3)(void *,void *,void *);
typedef int (*Fun4)(void *,void *,void *,void *);
typedef int (*Fun5)(void *,void *,void *,void *,void *);
typedef int (*Fun6)(void *,void *,void *,void *,void *,void *);
typedef int (*Fun7)(void *,void *,void *,void *,void *,void *,void *);
typedef int (*Fun8)(void *,void *,void *,void *,void *,void *,void *,void *);
typedef int (*Fun9)(void *,void *,void *,void *,void *,void *,void *,void *,void *);
typedef int (*Fun10)(void *,void *,void *,void *,void *,void *,void *,void *,void *,void *);


int LinuxCall0(void * addr){
   return ((Fun0)addr)();
}
int LinuxCall1(void * addr, void * a1){
   return ((Fun1)addr)(a1);
}
int LinuxCall2(void * addr, void * a1,void * a2){
   return ((Fun2)addr)(a1,a2);
}
int LinuxCall3(void * addr, void * a1,void * a2,void * a3){
   return ((Fun3)addr)(a1,a2,a3);
}
int LinuxCall4(void * addr, void * a1,void * a2,void * a3,void * a4){
   return ((Fun4)addr)(a1,a2,a3,a4);
}
int LinuxCall5(void * addr, void * a1,void * a2,void * a3,void * a4,void * a5){
   return ((Fun5)addr)(a1,a2,a3,a4,a5);
}
int LinuxCall6(void * addr, void * a1,void * a2,void * a3,void * a4,void * a5,void * a6){
   return ((Fun6)addr)(a1,a2,a3,a4,a5,a6);
}
int LinuxCall7(void * addr, void * a1,void * a2,void * a3,void * a4,void * a5,void * a6,void * a7){
   return ((Fun7)addr)(a1,a2,a3,a4,a5,a6,a7);
}
int LinuxCall8(void * addr, void * a1,void * a2,void * a3,void * a4,void * a5,void * a6,void * a7,void * a8){
   return ((Fun8)addr)(a1,a2,a3,a4,a5,a6,a7,a8);
}
int LinuxCall9(void * addr, void * a1,void * a2,void * a3,void * a4,void * a5,void * a6,void * a7,void * a8,void * a9){
   return ((Fun9)addr)(a1,a2,a3,a4,a5,a6,a7,a8,a9);
}
int LinuxCall10(void * addr, void * a1,void * a2,void * a3,void * a4,void * a5,void * a6,void * a7,void * a8,void * a9,void * a10){
   return ((Fun10)addr)(a1,a2,a3,a4,a5,a6,a7,a8,a9,a10);
}