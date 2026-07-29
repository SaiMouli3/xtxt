# xtxt — C implementation.
#
# `make test` builds with warnings as errors and runs both the unit checks and
# the shared conformance suite. `make asan` re-runs them under AddressSanitizer
# and UndefinedBehaviorSanitizer: this is the one implementation where a bug is
# a memory-safety bug, so the sanitizers are part of the test story, not an
# optional extra.

CC      ?= cc
CFLAGS  ?= -std=c99 -O2 -Wall -Wextra -Werror -Wshadow -Wconversion -Wsign-conversion \
           -Wstrict-prototypes -Wmissing-prototypes -pedantic
BUILD   := build

.PHONY: all test asan clean

all: $(BUILD)/libxtxt.a

$(BUILD):
	mkdir -p $(BUILD)

$(BUILD)/xtxt.o: xtxt.c xtxt.h | $(BUILD)
	$(CC) $(CFLAGS) -c xtxt.c -o $@

$(BUILD)/libxtxt.a: $(BUILD)/xtxt.o
	ar rcs $@ $<

$(BUILD)/unit: tests/unit.c $(BUILD)/libxtxt.a
	$(CC) $(CFLAGS) tests/unit.c -o $@ -L$(BUILD) -lxtxt

$(BUILD)/emit_canonical: tests/emit_canonical.c $(BUILD)/libxtxt.a
	$(CC) $(CFLAGS) tests/emit_canonical.c -o $@ -L$(BUILD) -lxtxt

test: $(BUILD)/unit $(BUILD)/emit_canonical
	./$(BUILD)/unit
	python3 tests/run_conformance.py

asan: clean
	$(MAKE) test CFLAGS="-std=c99 -O1 -g -Wall -Wextra -Werror -fsanitize=address,undefined -fno-omit-frame-pointer"

clean:
	rm -rf $(BUILD)
