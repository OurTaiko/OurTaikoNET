import js from '@eslint/js'
import tseslint from 'typescript-eslint'
import hooks from 'eslint-plugin-react-hooks'
import query from '@tanstack/eslint-plugin-query'
export default tseslint.config(
  // TJARenderer is another project's source, kept as it came.
  { ignores: ['dist', 'node_modules', 'src/TJARenderer'] },
  {
    files: ['src/**/*.{ts,tsx}'],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    plugins: { 'react-hooks': hooks, '@tanstack/query': query },
    rules: {
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'error',
      ...query.configs['flat/recommended'][0].rules,
      // A query's address is made from what is in its key, and taken when the query is made
      // because the source on show can change before the answer comes.
      '@tanstack/query/exhaustive-deps': 'off',
      // What the server says is read through a query (src/kit/query/client.ts).
      'no-restricted-syntax': [
        'error',
        {
          selector: "CallExpression[callee.name='useEffect'] CallExpression[callee.name='api']",
          message: 'Read through useQuery; a write belongs in an event handler or a mutation.',
        },
      ],
    },
  },
)
