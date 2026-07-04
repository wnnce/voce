import tseslint from 'typescript-eslint'

export default tseslint.config(
  {
    ignores: ['dist/', 'node_modules/', 'src/proto/'],
  },
  ...tseslint.configs.recommended,
)
