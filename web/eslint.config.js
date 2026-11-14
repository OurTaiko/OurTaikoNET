import js from '@eslint/js'
import tseslint from 'typescript-eslint'
import hooks from 'eslint-plugin-react-hooks'
export default tseslint.config(
  // TJARenderer is another project's source, kept as it came.
  { ignores: ['dist', 'node_modules', 'src/TJARenderer'] },
  {
    files: ['src/**/*.{ts,tsx}'],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    plugins: { 'react-hooks': hooks },
    rules: { 'react-hooks/rules-of-hooks': 'error', 'react-hooks/exhaustive-deps': 'error' },
  },
)
