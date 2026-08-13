package com.lokiscale.loomspan.internal.observability.web;

import org.springframework.http.server.PathContainer;
import org.springframework.core.io.Resource;
import org.springframework.http.HttpMethod;
import org.springframework.web.servlet.HandlerMapping;
import org.springframework.web.servlet.function.HandlerFunction;
import org.springframework.web.servlet.function.RequestPredicate;
import org.springframework.web.servlet.function.RequestPredicates;
import org.springframework.web.servlet.function.RouterFunction;
import org.springframework.web.servlet.function.RouterFunctions;
import org.springframework.web.servlet.function.ServerRequest;
import org.springframework.web.servlet.function.support.RouterFunctionMapping;
import org.springframework.web.servlet.handler.AbstractUrlHandlerMapping;
import org.springframework.web.servlet.mvc.method.RequestMappingInfo;
import org.springframework.web.servlet.mvc.method.annotation.RequestMappingHandlerMapping;
import org.springframework.web.servlet.resource.ResourceHttpRequestHandler;
import org.springframework.web.util.pattern.PathPattern;
import org.springframework.web.util.pattern.PathPatternParser;

import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Set;

public final class ObservabilityRouteCollisionDetector
{
    private final PathPatternParser parser = new PathPatternParser();
    private final List<HandlerMapping> handlerMappings;

    public ObservabilityRouteCollisionDetector(List<HandlerMapping> handlerMappings)
    {
        this.handlerMappings = List.copyOf(handlerMappings);
    }

    public boolean hasCollision()
    {
        for (HandlerMapping mapping : handlerMappings)
        {
            if (mapping instanceof RequestMappingHandlerMapping requestMappings
                    && annotatedCollision(requestMappings))
            {
                return true;
            }
            if (mapping instanceof AbstractUrlHandlerMapping urlMappings
                    && explicitUrlCollision(urlMappings))
            {
                return true;
            }
            if (mapping instanceof RouterFunctionMapping routerMappings
                    && functionalCollision(routerMappings.getRouterFunction()))
            {
                return true;
            }
            if (!(mapping instanceof RequestMappingHandlerMapping)
                    && !(mapping instanceof AbstractUrlHandlerMapping)
                    && !(mapping instanceof RouterFunctionMapping))
            {
                return true;
            }
        }
        return false;
    }

    private boolean annotatedCollision(RequestMappingHandlerMapping mappings)
    {
        for (RequestMappingInfo mapping : mappings.getHandlerMethods().keySet())
        {
            for (String patternValue : mapping.getPatternValues())
            {
                if (overlaps(patternValue)) return true;
            }
        }
        return false;
    }

    private boolean explicitUrlCollision(AbstractUrlHandlerMapping mappings)
    {
        for (Map.Entry<String, Object> entry : mappings.getHandlerMap().entrySet())
        {
            if (entry.getValue() instanceof ResourceHttpRequestHandler)
            {
                if (dedicatedResourceMapping(entry.getKey())) return true;
            }
            else if (overlaps(entry.getKey())) return true;
        }
        for (Map.Entry<PathPattern, Object> entry : mappings.getPathPatternHandlerMap().entrySet())
        {
            String pattern = entry.getKey().getPatternString();
            if (entry.getValue() instanceof ResourceHttpRequestHandler)
            {
                if (dedicatedResourceMapping(pattern)) return true;
            }
            else if (overlaps(pattern)) return true;
        }
        return false;
    }

    private static boolean dedicatedResourceMapping(String pattern)
    {
        if (pattern.equals("/**") || pattern.equals("/*") || pattern.equals("/{*path}"))
        {
            return false;
        }
        return mayOverlapNamespace(pattern);
    }

    private boolean functionalCollision(RouterFunction<?> router)
    {
        if (router == null) return false;
        CollisionVisitor visitor = new CollisionVisitor();
        router.accept(visitor);
        return visitor.collision;
    }

    private boolean overlaps(String patternValue)
    {
        try
        {
            PathPattern pattern = parser.parse(patternValue);
            PathContainer root = PathContainer.parsePath(ObservabilityApiPaths.ROOT);
            PathContainer child = PathContainer.parsePath(ObservabilityApiPaths.ROOT + "/reserved-probe");
            return pattern.matches(root) || pattern.matches(child)
                    || startsWithReservedNamespace(patternValue)
                    || mayOverlapNamespace(patternValue);
        }
        catch (RuntimeException ex)
        {
            return true;
        }
    }

    private static boolean startsWithReservedNamespace(String pattern)
    {
        return pattern.equalsIgnoreCase(ObservabilityApiPaths.ROOT)
                || pattern.length() > ObservabilityApiPaths.ROOT.length()
                && pattern.charAt(ObservabilityApiPaths.ROOT.length()) == '/'
                && pattern.regionMatches(true, 0, ObservabilityApiPaths.ROOT, 0,
                        ObservabilityApiPaths.ROOT.length());
    }

    private static boolean mayOverlapNamespace(String pattern)
    {
        String candidatePath = pattern.startsWith("/") ? pattern.substring(1) : pattern;
        if (candidatePath.isEmpty())
        {
            return false;
        }
        String[] candidate = candidatePath.split("/");
        String[] reserved = ObservabilityApiPaths.ROOT.substring(1).split("/");
        int shared = Math.min(candidate.length, reserved.length);
        for (int index = 0; index < shared; index++)
        {
            String segment = candidate[index];
            boolean dynamic = segment.indexOf('{') >= 0 || segment.indexOf('*') >= 0
                    || segment.indexOf('?') >= 0;
            if (!dynamic && !segment.equalsIgnoreCase(reserved[index]))
            {
                return false;
            }
        }
        if (candidate.length >= reserved.length)
        {
            return true;
        }
        String last = candidate[candidate.length - 1];
        return last.indexOf('*') >= 0 || last.startsWith("{*");
    }

    private static List<String> pathPatterns(RequestPredicate predicate)
    {
        PredicatePathVisitor visitor = new PredicatePathVisitor();
        predicate.accept(visitor);
        return visitor.pathPatterns();
    }

    private final class CollisionVisitor implements RouterFunctions.Visitor
    {
        private final ArrayDeque<List<String>> nested = new ArrayDeque<>();
        private boolean collision;

        @Override
        public void startNested(RequestPredicate predicate)
        {
            List<String> paths = pathPatterns(predicate);
            if (paths.isEmpty())
            {
                collision = true;
            }
            nested.addLast(paths.isEmpty() ? List.of("") : paths);
        }

        @Override
        public void endNested(RequestPredicate predicate)
        {
            nested.removeLast();
        }

        @Override
        public void route(RequestPredicate predicate, HandlerFunction<?> handlerFunction)
        {
            List<String> leaves = pathPatterns(predicate);
            if (leaves.isEmpty())
            {
                collision = true;
                return;
            }
            List<String> prefixes = List.of("");
            for (List<String> nestedPaths : nested)
            {
                List<String> combined = new ArrayList<>(prefixes.size() * nestedPaths.size());
                for (String prefix : prefixes)
                {
                    for (String nestedPath : nestedPaths)
                    {
                        combined.add(prefix + nestedPath);
                    }
                }
                prefixes = combined;
            }
            for (String prefix : prefixes)
            {
                for (String leaf : leaves)
                {
                    collision |= overlaps((prefix + leaf).replace("//", "/"));
                }
            }
        }

        @Override
        public void resources(java.util.function.Function<ServerRequest, Optional<Resource>> lookupFunction)
        {
            collision = true;
        }

        @Override
        public void attributes(Map<String, Object> attributes)
        {
        }

        @Override
        public void unknown(RouterFunction<?> routerFunction)
        {
            collision = true;
        }
    }

    private static final class PredicatePathVisitor implements RequestPredicates.Visitor
    {
        private final ArrayDeque<Frame> frames = new ArrayDeque<>();
        private PathResult result;

        List<String> pathPatterns()
        {
            return result == null || result.unclassifiable() || result.unconstrained()
                    ? List.of()
                    : result.paths();
        }

        @Override
        public void method(Set<HttpMethod> methods)
        {
            accept(PathResult.unconstrainedResult());
        }

        @Override
        public void path(String pattern)
        {
            accept(PathResult.path(pattern));
        }

        @Override
        public void pathExtension(String extension)
        {
            accept(PathResult.unconstrainedResult());
        }

        @Override
        public void version(String version)
        {
            accept(PathResult.unconstrainedResult());
        }

        @Override
        public void header(String name, String value)
        {
            accept(PathResult.unconstrainedResult());
        }

        @Override
        public void param(String name, String value)
        {
            accept(PathResult.unconstrainedResult());
        }

        @Override
        public void startAnd()
        {
            frames.addLast(new Frame(Operator.AND));
        }

        @Override
        public void and()
        {
            separator();
        }

        @Override
        public void endAnd()
        {
            finish(Operator.AND);
        }

        @Override
        public void startOr()
        {
            frames.addLast(new Frame(Operator.OR));
        }

        @Override
        public void or()
        {
            separator();
        }

        @Override
        public void endOr()
        {
            finish(Operator.OR);
        }

        @Override
        public void startNegate()
        {
            frames.addLast(new Frame(Operator.NEGATE));
        }

        @Override
        public void endNegate()
        {
            if (frames.isEmpty())
            {
                result = PathResult.unclassifiableResult();
                return;
            }
            frames.removeLast();
            accept(PathResult.unclassifiableResult());
        }

        @Override
        public void unknown(RequestPredicate predicate)
        {
            accept(PathResult.unclassifiableResult());
        }

        private void separator()
        {
            if (frames.isEmpty())
            {
                result = PathResult.unclassifiableResult();
                return;
            }
            frames.getLast().rightSide = true;
        }

        private void finish(Operator expected)
        {
            if (frames.isEmpty())
            {
                result = PathResult.unclassifiableResult();
                return;
            }
            Frame frame = frames.removeLast();
            PathResult combined = frame.operator == expected && frame.left != null && frame.right != null
                    ? combine(frame.operator, frame.left, frame.right)
                    : PathResult.unclassifiableResult();
            accept(combined);
        }

        private void accept(PathResult value)
        {
            if (frames.isEmpty())
            {
                result = result == null ? value : PathResult.unclassifiableResult();
                return;
            }
            Frame frame = frames.getLast();
            if (frame.rightSide)
            {
                frame.right = frame.right == null ? value : PathResult.unclassifiableResult();
            }
            else
            {
                frame.left = frame.left == null ? value : PathResult.unclassifiableResult();
            }
        }

        private static PathResult combine(Operator operator, PathResult left, PathResult right)
        {
            if (left.unclassifiable() || right.unclassifiable())
            {
                return PathResult.unclassifiableResult();
            }
            if (operator == Operator.OR)
            {
                if (left.unconstrained() || right.unconstrained())
                {
                    return PathResult.unconstrainedResult();
                }
                List<String> alternatives = new ArrayList<>(left.paths());
                right.paths().stream().filter(path -> !alternatives.contains(path)).forEach(alternatives::add);
                return new PathResult(false, false, List.copyOf(alternatives));
            }
            if (left.unconstrained()) return right;
            if (right.unconstrained()) return left;
            return left.paths().equals(right.paths()) ? left : PathResult.unclassifiableResult();
        }

        private enum Operator { AND, OR, NEGATE }

        private static final class Frame
        {
            private final Operator operator;
            private PathResult left;
            private PathResult right;
            private boolean rightSide;

            private Frame(Operator operator)
            {
                this.operator = operator;
            }
        }

        private record PathResult(boolean unclassifiable, boolean unconstrained, List<String> paths)
        {
            private static PathResult path(String value)
            {
                return new PathResult(false, false, List.of(value));
            }

            private static PathResult unconstrainedResult()
            {
                return new PathResult(false, true, List.of());
            }

            private static PathResult unclassifiableResult()
            {
                return new PathResult(true, false, List.of());
            }
        }
    }
}
