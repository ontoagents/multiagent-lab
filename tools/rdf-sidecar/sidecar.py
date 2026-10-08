#!/usr/bin/env python3
"""rdf-sidecar: OWL/RDF ↔ spec_json 双向转换（方案 04 v0.6 §2.2/§6，不自研 RDF 解析）。

用法:
  sidecar.py parse  --format owl_rdfxml|turtle|jsonld < input   -> stdout JSON {spec, warnings, lossy, note}
  sidecar.py export --ontology-id ID [--format turtle] < spec_json -> stdout TTL

导入映射（REQ-69 细则定稿，REQ-235/M62 扩 SKOS/JSON-LD/数据属性档；REQ-268/M77 数据属性声明捕获）:
  rdfs:label|skos:prefLabel（pref/alt 优先）→ name/label、rdfs:comment|skos:definition → definition、
  rdfs:subClassOf|skos:broader|skos:narrower(反向) → parents、
  owl:Class|rdfs:Class|skos:Concept → concept、
  owl:NamedIndividual+rdf:type → instance、对象属性断言 → 实例关系、
  datatype 属性断言 → attributes（实例级）；owl:DatatypeProperty 声明 → data_properties（REQ-268
  自「固定策略一档丢弃+warning」升档为**声明捕获**：name/label/definition/domain/range——
  domain 未注册为概念则留空不悬空，range xsd:* → 短名 string/number/integer/boolean/date；
  属性公理（函数性/限制等）仍丢弃 warning 明示）；其余公理/推理语义丢弃并计入 warnings。
导出 URN 规则与 pkg/ontology/spec 的 sanitize 逐字符一致（查询翻译共用）；
data_properties 声明导出为 <attr:名> owl:DatatypeProperty（与实例属性断言同 IRI，声明与使用统一）。
"""
import sys, json

try:
    from rdflib import Graph, RDF, RDFS, OWL, URIRef, Literal, SKOS
except ImportError:
    sys.stderr.write("rdflib 未安装：pip install rdflib\n")
    sys.exit(3)

RDFXML = "xml"
TTL = "turtle"
JSONLD = "json-ld"


def sanitize(s: str) -> str:
    return "".join(ch if (ch.isalnum() or ch in "_-.") else "_" for ch in (s or "").strip())


def local(uri: str) -> str:
    for sep in ("#", "/", ":"):
        if sep in uri:
            uri = uri.rsplit(sep, 1)[-1]
    return uri


def label_or_local(g, subj) -> str:
    # REQ-235①：SKOS prefLabel/altLabel 优先于 rdfs:label（SKOS 词表常无 rdfs:label）
    for pred in (SKOS.prefLabel, SKOS.altLabel, RDFS.label):
        for lab in g.objects(subj, pred):
            s = str(lab).strip()
            if s:
                return s
    return local(str(subj))


def definition_of(g, subj) -> str | None:
    # REQ-235①：skos:definition 与 rdfs:comment 等价承载定义
    for pred in (RDFS.comment, SKOS.definition):
        v = g.value(subj, pred)
        if v and str(v).strip():
            return str(v)
    return None


def label_of(g, subj) -> str | None:
    # REQ-268/M77：展示标签独立于名称（label_or_local 在无标签时回落 local，不适合判断「有无标签」）
    for pred in (SKOS.prefLabel, SKOS.altLabel, RDFS.label):
        for lab in g.objects(subj, pred):
            s = str(lab).strip()
            if s:
                return s
    return None


XSD_SHORT = {
    "http://www.w3.org/2001/XMLSchema#string": "string",
    "http://www.w3.org/2001/XMLSchema#double": "number",
    "http://www.w3.org/2001/XMLSchema#float": "number",
    "http://www.w3.org/2001/XMLSchema#decimal": "number",
    "http://www.w3.org/2001/XMLSchema#integer": "integer",
    "http://www.w3.org/2001/XMLSchema#int": "integer",
    "http://www.w3.org/2001/XMLSchema#long": "integer",
    "http://www.w3.org/2001/XMLSchema#boolean": "boolean",
    "http://www.w3.org/2001/XMLSchema#date": "date",
    "http://www.w3.org/2001/XMLSchema#dateTime": "date",
}


def range_short(rng) -> str:
    # REQ-268/M77：rdfs:range → 短名；未知 xsd/自定义 IRI 诚实保留原始 IRI（导出侧按原样回写）
    if rng is None:
        return "string"
    s = str(rng)
    return XSD_SHORT.get(s, s)


def parse(format: str) -> int:
    content = sys.stdin.read()
    g = Graph()
    fmt = {"owl_rdfxml": RDFXML, "turtle": TTL, "jsonld": JSONLD}.get(format)
    if fmt is None:
        sys.stderr.write(f"未知解析格式 {format}\n")
        return 2
    try:
        g.parse(data=content, format=fmt)
    except Exception as e:  # noqa: BLE001
        sys.stderr.write(f"RDF 解析失败: {e}\n")
        return 1

    warnings, lossy = [], False
    spec = {"name": "", "description": "", "concepts": [], "relations": [], "instances": []}

    # ---- 类：owl:Class / rdfs:Class / skos:Concept（REQ-235① SKOS 词表；跳过系统内建类）----
    seen_c = {}
    classes = set(g.subjects(RDF.type, OWL.Class)) | set(g.subjects(RDF.type, RDFS.Class)) | set(g.subjects(RDF.type, SKOS.Concept))
    for c in classes:
        if not isinstance(c, URIRef):
            continue
        uri = str(c)
        if uri.startswith(str(RDFS)) or uri.startswith(str(OWL)) or uri.startswith(str(RDF)) or uri.startswith(str(SKOS)):
            continue
        name = label_or_local(g, c)
        if name in seen_c:
            warnings.append(f"重复概念名 {name}（{uri}）已跳过")
            continue
        seen_c[uri] = name
        concept = {"name": name}
        comment = definition_of(g, c)
        if comment:
            concept["definition"] = comment
        parents = []
        for p in g.objects(c, RDFS.subClassOf):
            if isinstance(p, URIRef) and str(p) in seen_c:
                parents.append(seen_c[str(p)])
            elif isinstance(p, URIRef) and str(p) not in seen_c:
                # 父类尚未注册（外部类）——先记占位，后续第二轮补
                pass
            else:
                lossy = True
                warnings.append(f"{name} 的匿名父类（限制/公理）丢弃: {p.n3()[:80]}")
        # REQ-235①：skos:broader/narrower → parents 统一放第二轮（父注册完成后补，避免悬空引用）
        if parents:
            concept["parents"] = parents
        spec["concepts"].append(concept)

    # 第二轮补父类（前向引用；含 SKOS broader/narrower——REQ-235①）
    by_name = {c["name"]: c for c in spec["concepts"]}
    uri_by_name = {v: k for k, v in seen_c.items()}
    name_by_uri = {v: k for k, v in uri_by_name.items()}
    for c in classes:
        if not isinstance(c, URIRef) or str(c) not in seen_c:
            continue
        mine = by_name.get(seen_c[str(c)])
        if not mine:
            continue
        parents = []
        for p in g.objects(c, RDFS.subClassOf):
            if isinstance(p, URIRef):
                pname = name_by_uri.get(str(p))
                if pname and pname != mine["name"]:
                    parents.append(pname)
        # skos:broader → parents；skos:narrower 反向 → parents（目标须已注册为概念，外部引用 warning 丢弃）
        for p in list(g.objects(c, SKOS.broader)) + list(g.subjects(SKOS.narrower, c)):
            if not isinstance(p, URIRef):
                continue
            pname = name_by_uri.get(str(p))
            if pname and pname != mine["name"] and pname not in parents:
                parents.append(pname)
            elif pname is None and not parents:
                lossy = True
                warnings.append(f"{mine['name']} 的 skos:broader/narrower 目标 {local(str(p))} 未注册为概念，丢弃")
        if parents:
            mine["parents"] = parents

    # ---- 对象属性 ----
    seen_r = {}
    props = set(g.subjects(RDF.type, OWL.ObjectProperty))
    for pr in props:
        if not isinstance(pr, URIRef):
            continue
        uri = str(pr)
        if uri.startswith(str(RDF)) or uri.startswith(str(RDFS)) or uri.startswith(str(OWL)):
            continue
        name = label_or_local(g, pr)
        if name in seen_r:
            warnings.append(f"重复关系名 {name} 已跳过")
            continue
        seen_r[uri] = name
        rel = {"name": name}
        comment = g.value(pr, RDFS.comment)
        if comment:
            rel["definition"] = str(comment)
        dom = g.value(pr, RDFS.domain)
        rng = g.value(pr, RDFS.range)
        if dom is not None and str(dom) in seen_c:
            rel["from"] = seen_c[str(dom)]
        else:
            rel["from"] = ""
            lossy = True
            warnings.append(f"关系 {name} 定义域缺失或未注册为概念")
        if rng is not None and str(rng) in seen_c:
            rel["to"] = seen_c[str(rng)]
        else:
            rel["to"] = ""
            lossy = True
            warnings.append(f"关系 {name} 值域缺失或未注册为概念")
        spec["relations"].append(rel)
    # REQ-268/M77（D-O22 批次二）：owl:DatatypeProperty 声明捕获入模型——自 REQ-235⑥「丢弃+warning」升档。
    # name=label 优先（与概念/关系同 label_or_local 口径）；domain 未注册为概念则留空（不产生悬空引用）；
    # range xsd:* → 短名，未知保留 IRI；函数性/限制等属性公理仍丢弃 warning 明示。
    # 声明与实例 attributes 键同名关联：实例断言的谓词若命中声明 URI，键归一为声明名（label 可能≠URI local）。
    dt_props = set(g.subjects(RDF.type, OWL.DatatypeProperty))
    data_properties = []
    dp_name_by_uri = {}
    seen_dp = set()
    for pr in dt_props:
        if not isinstance(pr, URIRef) or str(pr).startswith((str(RDF), str(RDFS), str(OWL), str(SKOS))):
            continue
        name = label_or_local(g, pr)
        if name in seen_dp:
            warnings.append(f"重复数据属性名 {name} 已跳过")
            continue
        seen_dp.add(name)
        dp_name_by_uri[str(pr)] = name
        dp = {"name": name}
        lbl = label_of(g, pr)
        if lbl and lbl != name:
            dp["label"] = lbl
        comment = definition_of(g, pr)
        if comment:
            dp["definition"] = comment
        dom = g.value(pr, RDFS.domain)
        if dom is not None and str(dom) in seen_c:
            dp["domain"] = seen_c[str(dom)]
        elif dom is not None:
            warnings.append(f"数据属性 {name} 定义域 {label_or_local(g, dom)} 未注册为概念，留空")
        dp["range"] = range_short(g.value(pr, RDFS.range))
        data_properties.append(dp)
    if data_properties:
        spec["data_properties"] = data_properties

    # REQ-269/M78（D-O22 批次三）：类级公理捕获——owl:disjointWith / owl:equivalentClass。
    # 双端均已注册为概念才入 axioms（URI → 已注册概念名归一）；任一端未注册则 warning 丢弃
    # （防悬空引用，沿 SKOS broader/narrower 先例）；同 (type, subject) 多目标合并为一条。
    axioms = []
    ax_index = {}
    for pred, atype in ((OWL.disjointWith, "disjoint_with"), (OWL.equivalentClass, "equivalent_class")):
        for subj, obj in g.subject_objects(pred):
            if not isinstance(subj, URIRef) or not isinstance(obj, URIRef):
                continue  # 匿名/空白节点公理（限制类）仍丢弃 lossy，warning 于类遍历段已有口径
            s_name = seen_c.get(str(subj))
            o_name = seen_c.get(str(obj))
            if s_name is None or o_name is None:
                miss = s_name if s_name is None else o_name
                warnings.append(f"公理 {atype} 端点 {label_or_local(g, subj if s_name is None else obj)} 未注册为概念，丢弃")
                lossy = True
                _ = miss
                continue
            if s_name == o_name:
                continue
            key = (atype, s_name)
            if key in ax_index:
                if o_name not in axioms[ax_index[key]]["targets"]:
                    axioms[ax_index[key]]["targets"].append(o_name)
            else:
                ax_index[key] = len(axioms)
                axioms.append({"type": atype, "subject": s_name, "targets": [o_name]})
    if axioms:
        spec["axioms"] = axioms

    # ---- 实例 ----
    inst_by_uri = {}
    for ind in g.subjects(RDF.type, OWL.NamedIndividual):
        if not isinstance(ind, URIRef):
            continue
        uri = str(ind)
        name = label_or_local(g, ind)
        if name in inst_by_uri:
            warnings.append(f"重复实例名 {name} 已跳过")
            continue
        inst_by_uri[uri] = name
        it = {"name": name, "attributes": {}, "relations": []}
        ctypes = [str(t) for t in g.objects(ind, RDF.type)
                  if isinstance(t, URIRef) and str(t) in seen_c and str(t) != str(OWL.NamedIndividual)]
        if ctypes:
            it["concept"] = seen_c[ctypes[0]]
        else:
            # REQ-235：类型未注册为概念的实例丢弃（置空保留会产生过不了结构校验的悬空实例，
            # 卡死合并应用——与 GraphML「未标注概念跳过」同语义），warning 明示
            lossy = True
            warnings.append(f"实例 {name} 的类型未注册为概念，已丢弃（可先补概念定义后重新导入）")
            continue
        for p, o in g.predicate_objects(ind):
            pu, ou = str(p), str(o)
            if pu in (str(RDF.type), str(RDFS.label), str(RDFS.comment), str(SKOS.prefLabel), str(SKOS.altLabel), str(SKOS.definition)):
                continue
            if ou.startswith(str(RDF)) or ou.startswith(str(OWL)):
                continue
            if pu in seen_r:  # 对象属性 → 实例关系
                tgt = inst_by_uri.get(ou) or (str(o) and None)
                if tgt:
                    it["relations"].append({"rel": seen_r[pu], "target": tgt})
                else:
                    lossy = True
                    warnings.append(f"实例 {name} 关系 {seen_r[pu]} 的目标 {local(ou)} 不是已注册实例，丢弃")
            else:  # 其余断言 → attributes（REQ-268/M77：谓词命中数据属性声明时键归一为声明名）
                try:
                    val = o.toPython()
                    val = val.isoformat() if hasattr(val, "isoformat") else val
                except Exception:  # noqa: BLE001
                    val = ou
                it["attributes"][dp_name_by_uri.get(pu) or local(pu) or pu] = val
        if not it["attributes"]:
            it.pop("attributes")
        if not it["relations"]:
            it.pop("relations")
        spec["instances"].append(it)

    out = {"spec": spec, "warnings": warnings, "lossy": lossy,
           "note": "OWL/TTL/JSON-LD 有损导入（REQ-69 细则/REQ-235；REQ-268 数据属性声明捕获）：保留 类层次/SKOS 词表层次/对象属性/数据属性声明/实例断言，datatype 断言落实例 attributes；公理、限制、推理语义丢弃（计入 warnings）"}
    json.dump(out, sys.stdout, ensure_ascii=False)
    return 0


# ---- 导出：spec_json → TTL ----

def esc(s) -> str:
    return str(s).replace("\\", "\\\\").replace('"', '\\"')


def export(argv) -> int:
    oid = ""
    fmt = TTL
    args = argv
    i = 0
    while i < len(args):
        if args[i] == "--ontology-id":
            oid = args[i + 1]; i += 2
        elif args[i] == "--format":
            fmt = args[i + 1]; i += 2
        else:
            i += 1
    spec = json.load(sys.stdin)
    base = f"urn:o:{sanitize(oid)}:"
    cu = lambda n: f"{base}concept:{sanitize(n)}"       # noqa: E731
    ru = lambda n: f"{base}relation:{sanitize(n)}"      # noqa: E731
    iu = lambda n: f"{base}instance:{sanitize(n)}"      # noqa: E731
    au = lambda k: f"{base}attr:{sanitize(k)}"          # noqa: E731

    # REQ-268/M77：数据属性 range 短名 → xsd IRI（未知形态：http(s) IRI 原样 <尖括号>，其余回落 xsd:string）
    XSD_IRI = {
        "string": "xsd:string", "number": "xsd:double", "integer": "xsd:integer",
        "boolean": "xsd:boolean", "date": "xsd:date",
    }

    def range_iri(rng) -> str:
        r = (rng or "string").strip()
        if r in XSD_IRI:
            return XSD_IRI[r]
        if r.startswith("http://") or r.startswith("https://"):
            return f"<{r}>"
        return "xsd:string"

    L = []
    L.append(f'@prefix o: <{base}> .')
    L.append('@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .')
    L.append('@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .')
    L.append('@prefix owl: <http://www.w3.org/2002/07/owl#> .')
    L.append('@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .')
    L.append('')
    # REQ-216：空本体 spec（伴生绑定自动创建）concepts/relations/instances 可能为 null——
    # `or []` 容错（空 spec 导出为仅前缀的合法 TTL，宿主方案可正常 start/load）。
    for c in (spec.get("concepts") or []):
        u = cu(c["name"])
        L.append(f'<{u}> rdf:type owl:Class ; rdfs:label "{esc(c["name"])}"')
        if c.get("definition"):
            L[-1] += f' ;\n    rdfs:comment "{esc(c["definition"])}"'
        for p in c.get("parents", []):
            L[-1] += f' ;\n    rdfs:subClassOf <{cu(p)}>'
        L[-1] += ' .'
    for r in (spec.get("relations") or []):
        u = ru(r["name"])
        L.append(f'<{u}> rdf:type owl:ObjectProperty ; rdfs:label "{esc(r["name"])}"')
        if r.get("definition"):
            L[-1] += f' ;\n    rdfs:comment "{esc(r["definition"])}"'
        if r.get("from"):
            L[-1] += f' ;\n    rdfs:domain <{cu(r["from"])}>'
        if r.get("to"):
            L[-1] += f' ;\n    rdfs:range <{cu(r["to"])}>'
        L[-1] += ' .'
    for dp in (spec.get("data_properties") or []):
        # REQ-268/M77：声明导出为 <attr:名> owl:DatatypeProperty——与实例属性断言同 IRI（声明与使用统一）
        u = au(dp["name"])
        L.append(f'<{u}> rdf:type owl:DatatypeProperty ; rdfs:label "{esc(dp["name"])}"')
        if dp.get("definition"):
            L[-1] += f' ;\n    rdfs:comment "{esc(dp["definition"])}"'
        if dp.get("domain"):
            L[-1] += f' ;\n    rdfs:domain <{cu(dp["domain"])}>'
        L[-1] += f' ;\n    rdfs:range {range_iri(dp.get("range"))}'
        L[-1] += ' .'
    # REQ-269/M78：公理导出回写——<概念> owl:disjointWith / owl:equivalentClass <目标>（概念同 IRI 口径）
    for ax in (spec.get("axioms") or []):
        pred = "owl:disjointWith" if ax.get("type") == "disjoint_with" else "owl:equivalentClass"
        tgts = ", ".join(f"<{cu(t)}>" for t in (ax.get("targets") or []))
        if tgts:
            L.append(f'<{cu(ax["subject"])}> {pred} {tgts} .')
    for it in (spec.get("instances") or []):
        u = iu(it["name"])
        L.append(f'<{u}> rdf:type owl:NamedIndividual')
        if it.get("concept"):
            L[-1] += f' ;\n    rdf:type <{cu(it["concept"])}>'
        L[-1] += f' ;\n    rdfs:label "{esc(it["name"])}"'
        for k, v in (it.get("attributes") or {}).items():
            L[-1] += f' ;\n    <{au(k)}> "{esc(v)}"'
        for ir in it.get("relations", []):
            L[-1] += f' ;\n    <{ru(ir["rel"])}> <{iu(ir["target"])}>'
        L[-1] += ' .'

    ttl = "\n".join(L) + "\n"
    # rdflib 回读校验（导出即验证）
    try:
        g = Graph()
        g.parse(data=ttl, format=TTL)
    except Exception as e:  # noqa: BLE001
        sys.stderr.write(f"导出 TTL 校验失败: {e}\n")
        return 1
    sys.stdout.write(ttl)
    return 0


def main() -> int:
    if len(sys.argv) < 2:
        sys.stderr.write(__doc__)
        return 2
    cmd = sys.argv[1]
    if cmd == "parse":
        fmt = "turtle"
        if "--format" in sys.argv:
            fmt = sys.argv[sys.argv.index("--format") + 1]
        return parse(fmt)
    if cmd == "export":
        return export(sys.argv[2:])
    if cmd == "reason":
        # REQ-255②/M62 批次（60 号 H3）：推理级一致性检查档（owlrl OWL 2 RL 闭包；
        # 完整 DL 推理 HermiT 不引入——学习定位内 owlrl 够用）。输入 TTL，输出 JSON。
        fmt = "turtle"
        if "--format" in sys.argv:
            fmt = sys.argv[sys.argv.index("--format") + 1]
        return reason(fmt)
    sys.stderr.write(f"未知命令 {cmd}\n")
    return 2


def reason(format: str) -> int:
    content = sys.stdin.read()
    g = Graph()
    fmt = {"owl_rdfxml": RDFXML, "turtle": TTL, "jsonld": JSONLD}.get(format)
    if fmt is None:
        sys.stderr.write(f"未知推理检查格式 {format}\n")
        return 2
    try:
        g.parse(data=content, format=fmt)
    except Exception as e:  # noqa: BLE001
        sys.stderr.write(f"RDF 解析失败: {e}\n")
        return 1
    try:
        import owlrl  # 推理检查档依赖（REQ-255②；pip install owlrl）
        from owlrl.AxiomaticTriples import OWLRL_Axiomatic_Triples, OWLRL_D_Axiomatic_Triples
        sem = owlrl.OWLRL_Extension(g, OWLRL_Axiomatic_Triples, OWLRL_D_Axiomatic_Triples, rdfs=True)
        sem.closure()
        violations = [str(m) for m in sem.error_messages]
        consistent = len(violations) == 0
    except ImportError:
        sys.stderr.write("owlrl 未安装：pip install owlrl（推理检查档依赖）\n")
        return 3
    except Exception as e:  # noqa: BLE001
        # 推理器内部异常不伪装成「一致」——如实报 error 由调用方降级处理
        json.dump({"consistent": None, "violations": [], "error": f"owlrl 推理失败: {e}",
                   "note": "OWL 2 RL 闭包一致性检查（owlrl；HermiT 完整 DL 推理不引入）"}, sys.stdout, ensure_ascii=False)
        return 0
    json.dump({"consistent": consistent, "violations": violations[:50],
               "note": "OWL 2 RL 闭包一致性检查（owlrl；HermiT 完整 DL 推理不引入，60 号 H3 口径）"}, sys.stdout, ensure_ascii=False)
    return 0


if __name__ == "__main__":
    sys.exit(main())
