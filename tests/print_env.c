#include <stdio.h>

int main(int argc, char **argv, char **env) {
    for (int i = 0; env[i]; i += 1) {
        printf("[%d] %s\n", i, env[i]);
    }

    return 0;
}
