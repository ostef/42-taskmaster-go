#include <stdio.h>
#include <unistd.h>

int main() {
    char cwd[2000];
    getcwd(cwd, sizeof(cwd));

    printf("Working dir: %s\n", cwd);
}
