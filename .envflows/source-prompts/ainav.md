## AI Navigation System (.ainav)

The `.ainav/` directory contains a curated navigation map of the codebase. This is NOT source of truth (code is), but provides efficient navigation hints that should be consulted FIRST before exploring the codebase.

### How to Use .ainav

1. **Start at the entry point**: Read `.ainav/index.md` first
2. **Navigate by topic**: Follow links to subdirectory index files
3. **Max 3 hops**: Information is organized for quick access (index -> topic -> detail)

**Prefer `.ainav` first** when exploring or trying to understand the codebase. It often provides faster navigation with context. If the information there is insufficient, fall back to grep/glob/read as needed.

### Keeping .ainav Updated

**IMPORTANT**: When making code changes, update relevant `.ainav` files if:
- Adding new files/directories that affect navigation
- Changing file purposes or locations
- Adding new features that should be documented

Files should stay under 3KB. If a file grows too large, split into subdirectories.
Primary index file allowed to grow larger (up to ~10KB) for comprehensive overview.
