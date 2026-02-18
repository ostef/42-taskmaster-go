GO = go
NAME = taskmaster
SRC = ./src
RM = rm
TESTS_SOURCES=$(wildcard tests/*.c)
TESTS=$(TESTS_SOURCES:.c=)

all: all_tests build

build:
	$(GO) build -o $(NAME) $(SRC)

tests/%: tests/%.c
	$(CC) $< -o $@

all_tests: $(TESTS)

clean:
	$(GO) clean -cache

fclean: clean
	$(RM) $(NAME)
	$(RM) $(TESTS)

re: fclean all

.PHONY: clean fclean re all build all_tests
