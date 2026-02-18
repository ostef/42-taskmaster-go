#include <stdio.h>

int main(int argc, char **argv, char **env) {
    printf("Env: {\n");
    for (int i = 0; env[i]; i += 1) {
        printf("  [%d] %s\n", i, env[i]);
    }
    printf("}\n");

    return 0;
}
