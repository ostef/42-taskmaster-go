GO = go
NAME = taskmaster
SRC = ./src
RM = rm
TESTS_SOURCES=$(wildcard tests/*.c)
TESTS=$(TESTS_SOURCES:.c=)

all : build

build :
	$(GO) build -o $(NAME) $(SRC)

tests/%: tests/%.c
	$(CC) $< -o $@

all_tests: $(TESTS)

clean :
	$(RM) $(NAME)

fclean : clean
	$(GO) clean -cache
	$(RM) $(TESTS)

re : fclean all

.PHONY : clean fclean re all build all_tests
