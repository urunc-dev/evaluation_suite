#include <stdio.h>

// A simple C program that prints a message to the console and exits with a status code of 0.
// This program is used for benchmarking the lifecycle latency of urunc since 
// we cant rely on the `runtime.TaskStartEventTopic` from containerd since urunc releases
// the event before the program started in the cotainer.
int main(void) {
  printf("Hello, I have started\n");
  return 0;
}