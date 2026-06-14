import { create } from 'zustand'
import type { EvalSuite, TestCase, EvalRun } from '../api/types'

interface EvalStore {
  suites: EvalSuite[]
  activeSuite: EvalSuite | null
  testCases: TestCase[]
  activeRun: EvalRun | null

  setSuites: (suites: EvalSuite[]) => void
  setActiveSuite: (suite: EvalSuite | null) => void
  setTestCases: (cases: TestCase[]) => void
  setActiveRun: (run: EvalRun | null) => void

  upsertSuite: (suite: EvalSuite) => void
  removeSuite: (suiteId: string) => void
  upsertTestCase: (tc: TestCase) => void
  removeTestCase: (caseId: string) => void
}

export const useEvalStore = create<EvalStore>((set) => ({
  suites: [],
  activeSuite: null,
  testCases: [],
  activeRun: null,

  setSuites: (suites) => set({ suites }),
  setActiveSuite: (activeSuite) => set({ activeSuite }),
  setTestCases: (testCases) => set({ testCases }),
  setActiveRun: (activeRun) => set({ activeRun }),

  upsertSuite: (suite) =>
    set(s => {
      const idx = s.suites.findIndex(x => x.id === suite.id)
      const suites = idx >= 0
        ? s.suites.map(x => x.id === suite.id ? suite : x)
        : [suite, ...s.suites]
      return { suites, activeSuite: s.activeSuite?.id === suite.id ? suite : s.activeSuite }
    }),

  removeSuite: (suiteId) =>
    set(s => ({
      suites: s.suites.filter(x => x.id !== suiteId),
      activeSuite: s.activeSuite?.id === suiteId ? null : s.activeSuite,
    })),

  upsertTestCase: (tc) =>
    set(s => {
      const idx = s.testCases.findIndex(x => x.id === tc.id)
      const testCases = idx >= 0
        ? s.testCases.map(x => x.id === tc.id ? tc : x)
        : [...s.testCases, tc]
      return { testCases: [...testCases].sort((a, b) => a.position - b.position) }
    }),

  removeTestCase: (caseId) =>
    set(s => ({ testCases: s.testCases.filter(x => x.id !== caseId) })),
}))
