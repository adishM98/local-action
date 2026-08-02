import { Workflow, ShieldCheck, FlaskConical, Rocket, BookOpen, MoreHorizontal } from 'lucide-react'

export const CATEGORY_ORDER = ['CI/Build', 'Security', 'Testing', 'Deployment', 'Docs', 'Other']
export const CATEGORY_ICON = {
  'CI/Build': Workflow,
  Security: ShieldCheck,
  Testing: FlaskConical,
  Deployment: Rocket,
  Docs: BookOpen,
  Other: MoreHorizontal,
}
