GO = go
NAME = taskmaster
SRC = ./src
RM = rm

all : build

build :
	$(GO) build -o $(NAME) $(SRC)

clean :
	$(RM) $(NAME)

fclean : clean
	$(GO) clean -cache

re : fclean all

.PHONY : clean fclean re all build