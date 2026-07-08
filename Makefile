CC = gcc
CFLAGS = -Wall -Wextra -O2

MINIMAL_C_SRC = workloads/minimal-c/main.c
MINIMAL_C_TARGET = images/minimal-c/bin/minimal

all: $(MINIMAL_C_TARGET)

$(MINIMAL_C_TARGET): $(MINIMAL_C_SRC)
	mkdir -p images/minimal-c/bin
	$(CC) $(CFLAGS) -o $(MINIMAL_C_TARGET) $(MINIMAL_C_SRC)

clean:
	rm -rf images/minimal-c/bin
	rm -rf build

