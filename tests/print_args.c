#include <stdio.h>

int main(int argc, char **argv, char **env) {
    printf("Args: {\n");
    for (int i = 0; i < argc; i += 1) {
        printf("  [%d] %s\n", i, argv[i]);
    }
    printf("}\n");

    return 0;
}
